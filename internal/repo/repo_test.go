package repo

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sriharshini/aegis/internal/store"
)

const pass = "correct horse battery staple"

func setupRepo(t *testing.T) (*Repo, *store.LocalWORM, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewLocalWORM(filepath.Join(dir, "repo"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Init(context.Background(), st, pass)
	if err != nil {
		t.Fatal(err)
	}
	return r, st, filepath.Join(dir, "repo")
}

// makeCorpus writes n low-entropy "business" files (CSV / text).
func makeCorpus(t *testing.T, dir string, n int) {
	t.Helper()
	rng := mrand.New(mrand.NewSource(42))
	words := []string{"invoice", "patient", "claim", "amount", "approved", "pending", "Hyderabad", "2026", "total", "ledger"}
	for i := 0; i < n; i++ {
		var b strings.Builder
		for j := 0; j < 400+rng.Intn(400); j++ {
			fmt.Fprintf(&b, "%d,%s,%s,%d\n", j, words[rng.Intn(len(words))], words[rng.Intn(len(words))], rng.Intn(100000))
		}
		p := filepath.Join(dir, fmt.Sprintf("dept%d", i%4), fmt.Sprintf("file%03d.csv", i))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// simulateRansomware "encrypts" files in place with random-looking bytes,
// renames them with a .locked extension and drops a ransom note - the same
// observable behaviour as real families (LockBit, BlackCat), with none of the
// actual malware.
func simulateRansomware(t *testing.T, dir string, fraction float64) {
	t.Helper()
	var files []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	for i, p := range files {
		if float64(i) >= fraction*float64(len(files)) {
			break
		}
		fi, _ := os.Stat(p)
		junk := make([]byte, fi.Size())
		rand.Read(junk)
		os.WriteFile(p+".locked", junk, 0o644)
		os.Remove(p)
	}
	os.WriteFile(filepath.Join(dir, "HOW_TO_DECRYPT.txt"), []byte("pay us"), 0o644)
}

func dirsEqual(t *testing.T, a, b string) {
	t.Helper()
	filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, _ := filepath.Rel(a, p)
		x, _ := os.ReadFile(p)
		y, err := os.ReadFile(filepath.Join(b, rel))
		if err != nil || !bytes.Equal(x, y) {
			t.Fatalf("mismatch at %s: %v", rel, err)
		}
		return nil
	})
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 40)
	sn, err := r.Backup(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if sn.Verdict.Suspicious {
		t.Fatalf("clean first backup flagged: %+v", sn.Verdict)
	}
	dst := t.TempDir()
	if err := r.Restore(ctx, sn.ID, dst); err != nil {
		t.Fatal(err)
	}
	dirsEqual(t, src, dst)
}

func TestIncrementalDedup(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 40)
	first, _ := r.Backup(ctx, src)

	// Small edit to one file.
	p := filepath.Join(src, "dept0", "file000.csv")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, append([]byte("new header line\n"), b...), 0o644)

	second, err := r.Backup(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first uploaded %d bytes, second uploaded %d bytes of %d", first.Stats.NewBytes, second.Stats.NewBytes, second.Stats.Bytes)
	if second.Stats.NewBytes*20 > second.Stats.Bytes {
		t.Fatalf("incremental backup uploaded too much: %d of %d", second.Stats.NewBytes, second.Stats.Bytes)
	}
	if second.Verdict.Suspicious {
		t.Fatalf("a one-line edit was flagged as ransomware: %+v", second.Verdict)
	}
}

func TestRansomwareDetectedAndCleanRecovery(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 40)
	clean, _ := r.Backup(ctx, src)
	golden := t.TempDir()
	r.Restore(ctx, clean.ID, golden)

	simulateRansomware(t, src, 0.8)
	infected, err := r.Backup(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if !infected.Verdict.Suspicious {
		t.Fatalf("attack not detected: %+v", infected.Verdict)
	}
	t.Logf("verdict: %+v", infected.Verdict)

	// A second post-attack backup must still be flagged: it is compared
	// against the last CLEAN baseline, not the infected snapshot.
	again, _ := r.Backup(ctx, src)
	if !again.Verdict.Suspicious {
		t.Fatal("follow-up backup of encrypted data was treated as clean")
	}

	rp, err := r.LatestClean(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rp.ID != clean.ID {
		t.Fatalf("recovery point = %s, want %s", rp.ID, clean.ID)
	}
	dst := t.TempDir()
	if err := r.Restore(ctx, rp.ID, dst); err != nil {
		t.Fatal(err)
	}
	dirsEqual(t, golden, dst)
}

func TestWrongPassphraseRejected(t *testing.T) {
	_, st, _ := setupRepo(t)
	if _, err := Open(context.Background(), st, "nope"); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("expected ErrWrongKey, got %v", err)
	}
}

// Stealthy variant: encrypt in place, keep names, no ransom note. Only the
// entropy signal can catch this.
func TestInPlaceEncryptionDetected(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 40)
	r.Backup(ctx, src)
	n := 0
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && n < 20 {
			fi, _ := os.Stat(p)
			junk := make([]byte, fi.Size())
			rand.Read(junk)
			os.WriteFile(p, junk, 0o644)
			n++
		}
		return nil
	})
	sn, _ := r.Backup(ctx, src)
	t.Logf("verdict: %+v", sn.Verdict)
	if !sn.Verdict.Suspicious {
		t.Fatal("in-place encryption of 50% of files not detected")
	}
}

// A legitimate bulk change (every CSV gets a new column) must not trigger.
func TestBulkLegitimateEditNotFlagged(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 40)
	r.Backup(ctx, src)
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			b, _ := os.ReadFile(p)
			os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "\n", ",reviewed\n")), 0o644)
		}
		return nil
	})
	sn, _ := r.Backup(ctx, src)
	t.Logf("verdict: %+v", sn.Verdict)
	if sn.Verdict.Suspicious {
		t.Fatal("legitimate bulk edit flagged as ransomware (false positive)")
	}
}

func TestStoreIsWriteOnce(t *testing.T) {
	ctx := context.Background()
	r, st, _ := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 5)
	sn, _ := r.Backup(ctx, src)
	key := "snapshots/" + sn.ID
	if err := st.Put(ctx, key, []byte("overwrite")); !errors.Is(err, store.ErrExists) {
		t.Fatalf("overwrite allowed: %v", err)
	}
	if err := st.Delete(ctx, key); !errors.Is(err, store.ErrImmutable) {
		t.Fatalf("delete inside retention allowed: %v", err)
	}
}

func TestTamperedChunkDetected(t *testing.T) {
	ctx := context.Background()
	r, _, dir := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 5)
	sn, _ := r.Backup(ctx, src)

	// An attacker with raw disk access flips one byte of one chunk.
	id := sn.Files[0].Chunks[0]
	p := filepath.Join(dir, "chunks", id[:2], id)
	os.Chmod(p, 0o644)
	b, _ := os.ReadFile(p)
	b[len(b)/2] ^= 0xFF
	os.WriteFile(p, b, 0o644)

	rep, err := r.Verify(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) == 0 {
		t.Fatal("deep verify missed a tampered chunk")
	}
	if err := r.Restore(ctx, sn.ID, t.TempDir()); err == nil {
		t.Fatal("restore of tampered data succeeded")
	}
}

func TestDeletedSnapshotBreaksChain(t *testing.T) {
	ctx := context.Background()
	r, _, dir := setupRepo(t)
	src := t.TempDir()
	makeCorpus(t, src, 5)
	r.Backup(ctx, src)
	mid, _ := r.Backup(ctx, src)
	r.Backup(ctx, src)

	// Attacker bypasses the API and deletes a middle snapshot on disk.
	os.Remove(filepath.Join(dir, "snapshots", mid.ID))
	rep, err := r.Verify(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) == 0 {
		t.Fatal("verify did not notice a deleted snapshot")
	}
	t.Log(rep.Problems)
}
