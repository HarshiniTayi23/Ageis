//go:build integration

// Integration tests against a real S3-compatible server with Object Lock.
// Run with the local stack up:
//
//	make up && make integration
package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/HarshiniTayi23/Ageis/internal/store"
)

func s3Store(t *testing.T) *store.S3 {
	t.Helper()
	endpoint := os.Getenv("AEGIS_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("AEGIS_S3_ENDPOINT not set; start the stack with `make up`")
	}
	st, err := store.NewS3(store.S3Config{
		Endpoint:  endpoint,
		Bucket:    os.Getenv("AEGIS_BUCKET"),
		Creds:     store.Credentials{AccessKey: os.Getenv("AWS_ACCESS_KEY_ID"), SecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY")},
		PathStyle: true,
		Retention: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// prefixed scopes each test run to its own key prefix in the shared bucket,
// because locked objects from earlier runs cannot be cleaned up.
type prefixed struct {
	store.ObjectStore
	p string
}

func (s prefixed) Put(ctx context.Context, k string, d []byte) error {
	return s.ObjectStore.Put(ctx, s.p+k, d)
}
func (s prefixed) Get(ctx context.Context, k string) ([]byte, error) {
	return s.ObjectStore.Get(ctx, s.p+k)
}
func (s prefixed) Exists(ctx context.Context, k string) (bool, error) {
	return s.ObjectStore.Exists(ctx, s.p+k)
}
func (s prefixed) Delete(ctx context.Context, k string) error {
	return s.ObjectStore.Delete(ctx, s.p+k)
}
func (s prefixed) List(ctx context.Context, prefix string) ([]string, error) {
	keys, err := s.ObjectStore.List(ctx, s.p+prefix)
	for i := range keys {
		keys[i] = keys[i][len(s.p):]
	}
	return keys, err
}

func TestIntegrationS3BackupRestoreAndAttack(t *testing.T) {
	ctx := context.Background()
	st := prefixed{s3Store(t), fmt.Sprintf("it-%d/", time.Now().UnixNano())}

	r, err := Init(ctx, st, pass)
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	makeCorpus(t, src, 20)
	clean, err := r.Backup(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("backed up %d files, %d chunks to S3", clean.Stats.Files, clean.Stats.Chunks)

	// The attacker holds the S3 root credentials and goes for the backups.
	key := "snapshots/" + clean.ID
	if err := st.Put(ctx, key, []byte("garbage")); !errors.Is(err, store.ErrExists) {
		t.Fatalf("overwrite of snapshot: got %v, want ErrExists", err)
	}
	if err := st.Delete(ctx, key); !errors.Is(err, store.ErrImmutable) {
		t.Fatalf("delete of snapshot: got %v, want ErrImmutable", err)
	}
	chunk := "chunks/" + clean.Files[0].Chunks[0][:2] + "/" + clean.Files[0].Chunks[0]
	if err := st.Delete(ctx, chunk); !errors.Is(err, store.ErrImmutable) {
		t.Fatalf("delete of chunk: got %v, want ErrImmutable", err)
	}

	// Then encrypts production; the next backup is flagged.
	simulateRansomware(t, src, 0.8)
	infected, err := r.Backup(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if !infected.Verdict.Suspicious {
		t.Fatalf("attack not detected: %+v", infected.Verdict)
	}

	// Recovery: the clean point is found and restored, fully verified.
	rp, err := r.LatestClean(ctx)
	if err != nil || rp.ID != clean.ID {
		t.Fatalf("latest clean = %v, %v; want %s", rp, err, clean.ID)
	}
	if err := r.Restore(ctx, rp.ID, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	rep, err := r.Verify(ctx, true)
	if err != nil || len(rep.Problems) != 0 {
		t.Fatalf("verify: %v %v", err, rep)
	}
	drill, err := r.Drill(ctx)
	if err != nil || !drill.Passed {
		t.Fatalf("drill: %v %+v", err, drill)
	}
}
