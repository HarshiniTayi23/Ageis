// Package repo is the backup engine: it turns a directory tree into an
// encrypted, deduplicated, tamper-evident snapshot in an ObjectStore, and
// turns a snapshot back into files.
//
// Object layout in the store:
//
//	config                       plaintext: format version + KDF salt
//	keycheck                     sealed known value (detects wrong passphrase)
//	chunks/ab/abcdef...          sealed chunk, key = HMAC content address
//	snapshots/<unixnano>-<rand>  sealed JSON manifest
//
// Every snapshot manifest records the SHA-256 of the previous manifest, so the
// snapshot history forms a hash chain: silently deleting or editing a past
// snapshot breaks the chain and `verify` reports it.
package repo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sriharshini/aegis/internal/chunker"
	"github.com/sriharshini/aegis/internal/detect"
	"github.com/sriharshini/aegis/internal/seal"
	"github.com/sriharshini/aegis/internal/store"
)

const formatVersion = 1

var (
	ErrWrongKey     = errors.New("repo: wrong passphrase")
	ErrNoSnapshots  = errors.New("repo: no snapshots")
	ErrNoCleanPoint = errors.New("repo: no clean snapshot found")
)

type config struct {
	Version int       `json:"version"`
	Salt    []byte    `json:"salt"`
	Created time.Time `json:"created"`
}

// FileEntry describes one file inside a snapshot.
type FileEntry struct {
	detect.FileStat
	Mode    fs.FileMode `json:"mode"`
	ModTime time.Time   `json:"mtime"`
	Chunks  []string    `json:"chunks"`
}

// Stats summarises the work done for one backup.
type Stats struct {
	Files      int     `json:"files"`
	Bytes      int64   `json:"bytes"`
	Chunks     int     `json:"chunks"`
	NewChunks  int64   `json:"new_chunks"`
	NewBytes   int64   `json:"new_bytes"` // plaintext bytes actually uploaded
	DurationMS int64   `json:"duration_ms"`
	DedupRatio float64 `json:"dedup_ratio"` // Bytes / NewBytes
}

// Snapshot is a point-in-time manifest.
type Snapshot struct {
	ID       string         `json:"id"`
	Time     time.Time      `json:"time"`
	Source   string         `json:"source"`
	Parent   string         `json:"parent,omitempty"`    // previous snapshot (hash chain)
	PrevHash string         `json:"prev_hash,omitempty"` // sha256 of parent manifest
	Baseline string         `json:"baseline,omitempty"`  // clean snapshot used for detection
	Files    []FileEntry    `json:"files"`
	Stats    Stats          `json:"stats"`
	Verdict  detect.Verdict `json:"verdict"`
}

// Repo is an open repository.
type Repo struct {
	st      store.ObjectStore
	keys    *seal.Keyring
	Workers int
	Thresh  detect.Thresholds
}

// Init creates a new repository in st.
func Init(ctx context.Context, st store.ObjectStore, passphrase string) (*Repo, error) {
	salt, err := seal.NewSalt()
	if err != nil {
		return nil, err
	}
	cfg, _ := json.Marshal(config{Version: formatVersion, Salt: salt, Created: time.Now().UTC()})
	if err := st.Put(ctx, "config", cfg); err != nil {
		if errors.Is(err, store.ErrExists) {
			return nil, errors.New("repo: already initialised")
		}
		return nil, err
	}
	keys, err := seal.DeriveKeyring(passphrase, salt)
	if err != nil {
		return nil, err
	}
	kc, err := keys.Seal("keycheck", []byte("aegis"))
	if err != nil {
		return nil, err
	}
	if err := st.Put(ctx, "keycheck", kc); err != nil {
		return nil, err
	}
	return newRepo(st, keys), nil
}

// Open opens an existing repository.
func Open(ctx context.Context, st store.ObjectStore, passphrase string) (*Repo, error) {
	raw, err := st.Get(ctx, "config")
	if err != nil {
		return nil, fmt.Errorf("repo: reading config: %w", err)
	}
	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.Version != formatVersion {
		return nil, fmt.Errorf("repo: unsupported format version %d", cfg.Version)
	}
	keys, err := seal.DeriveKeyring(passphrase, cfg.Salt)
	if err != nil {
		return nil, err
	}
	kc, err := st.Get(ctx, "keycheck")
	if err != nil {
		return nil, err
	}
	if _, err := keys.Open("keycheck", kc); err != nil {
		return nil, ErrWrongKey
	}
	return newRepo(st, keys), nil
}

func newRepo(st store.ObjectStore, keys *seal.Keyring) *Repo {
	return &Repo{st: st, keys: keys, Workers: runtime.NumCPU() * 2, Thresh: detect.Default}
}

func chunkKey(id string) string { return "chunks/" + id[:2] + "/" + id }

// ---------------------------------------------------------------- backup ---

type upload struct {
	id   string
	data []byte
}

// Backup snapshots the directory tree at src.
func (r *Repo) Backup(ctx context.Context, src string) (*Snapshot, error) {
	start := time.Now()
	src, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Bounded worker pool: chunking is CPU-bound and runs on this goroutine,
	// while encryption + upload (network-bound for a cloud store) fan out.
	jobs := make(chan upload, r.Workers*2)
	var (
		wg        sync.WaitGroup
		errOnce   sync.Once
		uploadErr error
		inflight  sync.Map // dedup chunks seen within this run
		newChunks atomic.Int64
		newBytes  atomic.Int64
	)
	fail := func(err error) { errOnce.Do(func() { uploadErr = err; cancel() }) }
	for i := 0; i < r.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				if ctx.Err() != nil {
					continue // drain
				}
				if err := r.putChunk(ctx, u, &newChunks, &newBytes); err != nil {
					fail(err)
				}
			}
		}()
	}

	snap := &Snapshot{Time: time.Now().UTC(), Source: src}
	walkErr := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !d.Type().IsRegular() {
			return nil // directories are implied; symlinks/devices skipped in v1
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		fe, err := r.backupFile(ctx, p, filepath.ToSlash(rel), jobs, &inflight)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		snap.Files = append(snap.Files, *fe)
		snap.Stats.Files++
		snap.Stats.Bytes += fe.Size
		snap.Stats.Chunks += len(fe.Chunks)
		return nil
	})
	close(jobs)
	wg.Wait()
	if uploadErr != nil {
		return nil, uploadErr
	}
	if walkErr != nil {
		return nil, walkErr
	}

	// Hash-chain to the previous snapshot; detect against the last CLEAN one.
	all, err := r.Snapshots(ctx)
	if err != nil && !errors.Is(err, ErrNoSnapshots) {
		return nil, err
	}
	var baselineStats []detect.FileStat
	if len(all) > 0 {
		parent := all[len(all)-1]
		snap.Parent = parent.ID
		snap.PrevHash, err = r.manifestHash(ctx, parent.ID)
		if err != nil {
			return nil, err
		}
		if clean, err := latestClean(all); err == nil {
			snap.Baseline = clean.ID
			baselineStats = stats(clean.Files)
		}
	}
	if len(all) > 0 && snap.Baseline == "" {
		// Every prior snapshot is suspicious: compare against the most recent.
		baselineStats = stats(all[len(all)-1].Files)
	}
	snap.Verdict = detect.Analyze(baselineStats, stats(snap.Files), r.Thresh)

	snap.Stats.NewChunks = newChunks.Load()
	snap.Stats.NewBytes = newBytes.Load()
	snap.Stats.DurationMS = time.Since(start).Milliseconds()
	if snap.Stats.NewBytes > 0 {
		snap.Stats.DedupRatio = float64(snap.Stats.Bytes) / float64(snap.Stats.NewBytes)
	}

	snap.ID = newSnapshotID(snap.Time)
	body, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	key := "snapshots/" + snap.ID
	blob, err := r.keys.Seal(key, body)
	if err != nil {
		return nil, err
	}
	if err := r.st.Put(ctx, key, blob); err != nil {
		return nil, err
	}
	return snap, nil
}

func (r *Repo) backupFile(ctx context.Context, p, rel string, jobs chan<- upload, inflight *sync.Map) (*FileEntry, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fe := &FileEntry{Mode: fi.Mode().Perm(), ModTime: fi.ModTime().UTC()}
	fe.Path = rel

	h := sha256.New()
	var hist [256]uint64
	var total uint64
	c := chunker.New(f)
	for {
		chunk, err := c.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		h.Write(chunk)
		for _, b := range chunk {
			hist[b]++
		}
		total += uint64(len(chunk))
		id := r.keys.ID(chunk)
		fe.Chunks = append(fe.Chunks, id)
		if _, seen := inflight.LoadOrStore(id, struct{}{}); seen {
			continue
		}
		data := make([]byte, len(chunk)) // chunker reuses its buffer
		copy(data, chunk)
		select {
		case jobs <- upload{id: id, data: data}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	fe.Size = int64(total)
	fe.SHA256 = hex.EncodeToString(h.Sum(nil))
	fe.Entropy = detect.Entropy(&hist, total)
	return fe, nil
}

func (r *Repo) putChunk(ctx context.Context, u upload, n, b *atomic.Int64) error {
	key := chunkKey(u.id)
	ok, err := r.st.Exists(ctx, key)
	if err != nil {
		return err
	}
	if ok {
		return nil // deduplicated against a previous backup
	}
	blob, err := r.keys.Seal(key, u.data)
	if err != nil {
		return err
	}
	if err := r.st.Put(ctx, key, blob); err != nil && !errors.Is(err, store.ErrExists) {
		return err // ErrExists = a concurrent writer won the race; fine.
	}
	n.Add(1)
	b.Add(int64(len(u.data)))
	return nil
}

// --------------------------------------------------------------- catalog ---

// Snapshots returns all readable snapshots, oldest first. A manifest that
// fails authentication is skipped rather than failing the whole call: one
// tampered object must not stop tomorrow's backup from running. Verify
// reports the skipped ones.
func (r *Repo) Snapshots(ctx context.Context) ([]*Snapshot, error) {
	out, _, err := r.scan(ctx)
	return out, err
}

func (r *Repo) scan(ctx context.Context) ([]*Snapshot, []string, error) {
	keys, err := r.st.List(ctx, "snapshots/")
	if err != nil {
		return nil, nil, err
	}
	out := make([]*Snapshot, 0, len(keys))
	var bad []string
	for _, k := range keys {
		s, err := r.Snapshot(ctx, strings.TrimPrefix(k, "snapshots/"))
		if errors.Is(err, seal.ErrTampered) {
			bad = append(bad, fmt.Sprintf("%s: manifest failed authentication (tampered)", k))
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		out = append(out, s)
	}
	if len(out) == 0 && len(bad) == 0 {
		return nil, nil, ErrNoSnapshots
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, bad, nil
}

// Snapshot loads one snapshot by ID (a unique prefix is accepted).
func (r *Repo) Snapshot(ctx context.Context, id string) (*Snapshot, error) {
	body, err := r.manifest(ctx, id)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repo) manifest(ctx context.Context, id string) ([]byte, error) {
	key := "snapshots/" + id
	blob, err := r.st.Get(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		keys, lerr := r.st.List(ctx, key)
		if lerr != nil {
			return nil, lerr
		}
		if len(keys) != 1 {
			return nil, fmt.Errorf("repo: snapshot %q not found or ambiguous", id)
		}
		key = keys[0]
		blob, err = r.st.Get(ctx, key)
	}
	if err != nil {
		return nil, err
	}
	return r.keys.Open(key, blob)
}

func (r *Repo) manifestHash(ctx context.Context, id string) (string, error) {
	body, err := r.manifest(ctx, id)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// LatestClean returns the most recent snapshot not flagged as suspicious.
func (r *Repo) LatestClean(ctx context.Context) (*Snapshot, error) {
	all, err := r.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	return latestClean(all)
}

func latestClean(all []*Snapshot) (*Snapshot, error) {
	for i := len(all) - 1; i >= 0; i-- {
		if !all[i].Verdict.Suspicious {
			return all[i], nil
		}
	}
	return nil, ErrNoCleanPoint
}

func stats(files []FileEntry) []detect.FileStat {
	out := make([]detect.FileStat, len(files))
	for i, f := range files {
		out[i] = f.FileStat
	}
	return out
}

func newSnapshotID(t time.Time) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%019d-%s", t.UnixNano(), hex.EncodeToString(b))
}

// --------------------------------------------------------------- restore ---

// Restore writes snapshot id into dst, verifying every file's SHA-256.
func (r *Repo) Restore(ctx context.Context, id, dst string) error {
	snap, err := r.Snapshot(ctx, id)
	if err != nil {
		return err
	}
	dst, err = filepath.Abs(dst)
	if err != nil {
		return err
	}
	for _, fe := range snap.Files {
		target := filepath.Join(dst, filepath.FromSlash(fe.Path))
		// Defend against path traversal from a malicious manifest.
		if !strings.HasPrefix(target, dst+string(os.PathSeparator)) {
			return fmt.Errorf("repo: refusing to restore outside destination: %s", fe.Path)
		}
		if err := r.restoreFile(ctx, fe, target); err != nil {
			return fmt.Errorf("%s: %w", fe.Path, err)
		}
	}
	return nil
}

func (r *Repo) restoreFile(ctx context.Context, fe FileEntry, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".aegis-partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	w := io.MultiWriter(f, h)
	for _, id := range fe.Chunks {
		data, err := r.readChunk(ctx, id)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
		if _, err := w.Write(data); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != fe.SHA256 {
		os.Remove(tmp)
		return fmt.Errorf("integrity check failed: sha256 %s != %s", got, fe.SHA256)
	}
	if err := os.Chmod(tmp, fe.Mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	return os.Chtimes(target, fe.ModTime, fe.ModTime)
}

func (r *Repo) readChunk(ctx context.Context, id string) ([]byte, error) {
	key := chunkKey(id)
	blob, err := r.st.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", id[:12], err)
	}
	data, err := r.keys.Open(key, blob)
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", id[:12], err)
	}
	if r.keys.ID(data) != id {
		return nil, fmt.Errorf("chunk %s: content does not match address", id[:12])
	}
	return data, nil
}

// ---------------------------------------------------------------- verify ---

// VerifyReport is the result of a repository integrity check.
type VerifyReport struct {
	Snapshots     int      `json:"snapshots"`
	ChunksChecked int      `json:"chunks_checked"`
	Problems      []string `json:"problems,omitempty"`
}

// Verify checks the snapshot hash chain and, if deep, decrypts and
// re-addresses every referenced chunk.
func (r *Repo) Verify(ctx context.Context, deep bool) (*VerifyReport, error) {
	all, bad, err := r.scan(ctx)
	if err != nil {
		return nil, err
	}
	rep := &VerifyReport{Snapshots: len(all), Problems: bad}
	for i, s := range all {
		if i == 0 {
			if s.Parent != "" {
				rep.Problems = append(rep.Problems, fmt.Sprintf("snapshot %s: parent %s is missing (history truncated?)", s.ID, s.Parent))
			}
			continue
		}
		prev := all[i-1]
		if s.Parent != prev.ID {
			rep.Problems = append(rep.Problems, fmt.Sprintf("snapshot %s: chain break, expected parent %s got %s", s.ID, prev.ID, s.Parent))
			continue
		}
		h, err := r.manifestHash(ctx, prev.ID)
		if err != nil {
			return nil, err
		}
		if h != s.PrevHash {
			rep.Problems = append(rep.Problems, fmt.Sprintf("snapshot %s: parent manifest hash mismatch", s.ID))
		}
	}
	seen := map[string]bool{}
	for _, s := range all {
		for _, fe := range s.Files {
			for _, id := range fe.Chunks {
				if seen[id] {
					continue
				}
				seen[id] = true
				rep.ChunksChecked++
				if deep {
					if _, err := r.readChunk(ctx, id); err != nil {
						rep.Problems = append(rep.Problems, fmt.Sprintf("%s in %s: %v", fe.Path, s.ID, err))
					}
				} else if ok, err := r.st.Exists(ctx, chunkKey(id)); err != nil || !ok {
					rep.Problems = append(rep.Problems, fmt.Sprintf("%s in %s: chunk %s missing", fe.Path, s.ID, id[:12]))
				}
			}
		}
	}
	return rep, nil
}
