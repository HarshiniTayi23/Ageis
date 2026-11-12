package store

import (
	"context"
	"crypto/md5" //nolint:gosec // test mirrors the Content-MD5 check S3 does
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is a minimal versioned, object-locked S3 server. It checks every
// request's SigV4 signature server-side, so the tests prove the client signs
// exactly what it sends over the wire, including path encoding.
type fakeS3 struct {
	t        *testing.T
	bucket   string
	mu       sync.Mutex
	versions map[string][]fakeVersion // key -> versions, newest last
	nextID   int
	failNext int // respond 503 this many times (retry test)
}

type fakeVersion struct {
	id          string
	data        []byte
	retainUntil time.Time
}

func newFakeS3(t *testing.T) (*fakeS3, *S3) {
	f := &fakeS3{t: t, bucket: "vault", versions: map[string][]fakeVersion{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	st, err := NewS3(S3Config{
		Endpoint: srv.URL, Bucket: "vault", PathStyle: true,
		Creds:     Credentials{AccessKey: "test", SecretKey: "secret"},
		Retention: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return f, st
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext > 0 {
		f.failNext--
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := f.verifySignature(r); err != nil {
		f.t.Errorf("signature check failed for %s %s: %v", r.Method, r.URL, err)
		writeErr(w, 403, "SignatureDoesNotMatch", err.Error())
		return
	}
	prefix := "/" + f.bucket + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeErr(w, 404, "NoSuchBucket", r.URL.Path)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, prefix)
	body, _ := io.ReadAll(r.Body)

	switch {
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2":
		f.list(w, r)
	case r.Method == http.MethodPut:
		sum := md5.Sum(body) //nolint:gosec
		if r.Header.Get("Content-Md5") != base64.StdEncoding.EncodeToString(sum[:]) {
			writeErr(w, 400, "BadDigest", "content-md5 mismatch")
			return
		}
		if r.Header.Get("If-None-Match") == "*" && f.current(key) != nil {
			writeErr(w, 412, "PreconditionFailed", "exists")
			return
		}
		until, _ := time.Parse(time.RFC3339, r.Header.Get("X-Amz-Object-Lock-Retain-Until-Date"))
		f.nextID++
		f.versions[key] = append(f.versions[key], fakeVersion{id: fmt.Sprint(f.nextID), data: body, retainUntil: until})
		w.WriteHeader(200)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		v := f.current(key)
		if v == nil {
			writeErr(w, 404, "NoSuchKey", key)
			return
		}
		w.Header().Set("X-Amz-Version-Id", v.id)
		w.WriteHeader(200)
		if r.Method == http.MethodGet {
			_, _ = w.Write(v.data)
		}
	case r.Method == http.MethodDelete:
		vid := r.URL.Query().Get("versionId")
		for i, v := range f.versions[key] {
			if v.id == vid {
				if time.Now().Before(v.retainUntil) {
					writeErr(w, 400, "InvalidRequest", "Object is WORM protected and cannot be overwritten")
					return
				}
				f.versions[key] = append(f.versions[key][:i], f.versions[key][i+1:]...)
				w.WriteHeader(204)
				return
			}
		}
		writeErr(w, 404, "NoSuchVersion", vid)
	default:
		writeErr(w, 405, "MethodNotAllowed", r.Method)
	}
}

func (f *fakeS3) current(key string) *fakeVersion {
	vs := f.versions[key]
	if len(vs) == 0 {
		return nil
	}
	return &vs[len(vs)-1]
}

// list pages two keys at a time to exercise the client's pagination.
func (f *fakeS3) list(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	var keys []string
	for k := range f.versions {
		if strings.HasPrefix(k, prefix) && f.current(k) != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	start := 0
	if tok := r.URL.Query().Get("continuation-token"); tok != "" {
		_, _ = fmt.Sscan(tok, &start)
	}
	end := start + 2
	truncated := end < len(keys)
	if end > len(keys) {
		end = len(keys)
	}
	var b strings.Builder
	b.WriteString("<ListBucketResult>")
	for _, k := range keys[start:end] {
		fmt.Fprintf(&b, "<Contents><Key>%s</Key></Contents>", k)
	}
	fmt.Fprintf(&b, "<IsTruncated>%v</IsTruncated>", truncated)
	if truncated {
		fmt.Fprintf(&b, "<NextContinuationToken>%d</NextContinuationToken>", end)
	}
	b.WriteString("</ListBucketResult>")
	_, _ = w.Write([]byte(b.String()))
}

func (f *fakeS3) verifySignature(r *http.Request) error {
	auth := r.Header.Get("Authorization")
	i := strings.Index(auth, "Signature=")
	if i < 0 {
		return errors.New("missing signature")
	}
	got := auth[i+len("Signature="):]
	t, err := time.Parse(amzDateFormat, r.Header.Get("X-Amz-Date"))
	if err != nil {
		return err
	}
	// Re-sign a copy of the received request and compare.
	cp := r.Clone(context.Background())
	cp.Header.Del("Authorization")
	signV4(cp, Credentials{"test", "secret"}, "us-east-1", "s3", r.Header.Get("X-Amz-Content-Sha256"), t)
	want := cp.Header.Get("Authorization")
	if j := strings.Index(want, "Signature="); j < 0 || want[j+len("Signature="):] != got {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func writeErr(w http.ResponseWriter, code int, c, msg string) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "<Error><Code>%s</Code><Message>%s</Message></Error>", c, msg)
}

// ---------------------------------------------------------------- tests ---

func TestS3PutGetRoundTrip(t *testing.T) {
	_, st := newFakeS3(t)
	ctx := context.Background()
	if err := st.Put(ctx, "chunks/ab/abc$+ weird", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	b, err := st.Get(ctx, "chunks/ab/abc$+ weird")
	if err != nil || string(b) != "hello" {
		t.Fatalf("got %q, %v", b, err)
	}
	if ok, _ := st.Exists(ctx, "chunks/ab/abc$+ weird"); !ok {
		t.Fatal("exists = false")
	}
	if _, err := st.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing key: %v", err)
	}
}

func TestS3PutIsWriteOnce(t *testing.T) {
	_, st := newFakeS3(t)
	ctx := context.Background()
	_ = st.Put(ctx, "snapshots/1", []byte("original"))
	if err := st.Put(ctx, "snapshots/1", []byte("attacker")); !errors.Is(err, ErrExists) {
		t.Fatalf("overwrite: %v", err)
	}
	b, _ := st.Get(ctx, "snapshots/1")
	if string(b) != "original" {
		t.Fatalf("content changed to %q", b)
	}
}

func TestS3DeleteRefusedUnderRetention(t *testing.T) {
	_, st := newFakeS3(t)
	ctx := context.Background()
	_ = st.Put(ctx, "snapshots/1", []byte("x"))
	if err := st.Delete(ctx, "snapshots/1"); !errors.Is(err, ErrImmutable) {
		t.Fatalf("delete under retention: %v", err)
	}
}

func TestS3DeleteAllowedAfterRetention(t *testing.T) {
	_, st := newFakeS3(t)
	st.cfg.Retention = 0 // no retain-until date: nothing locked in the fake
	ctx := context.Background()
	_ = st.Put(ctx, "tmp/1", []byte("x"))
	if err := st.Delete(ctx, "tmp/1"); err != nil {
		t.Fatalf("delete after retention: %v", err)
	}
	if ok, _ := st.Exists(ctx, "tmp/1"); ok {
		t.Fatal("still exists")
	}
}

func TestS3ListPaginates(t *testing.T) {
	_, st := newFakeS3(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = st.Put(ctx, fmt.Sprintf("snapshots/%d", i), []byte("x"))
	}
	_ = st.Put(ctx, "chunks/zz", []byte("x"))
	keys, err := st.List(ctx, "snapshots/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 5 {
		t.Fatalf("got %d keys: %v", len(keys), keys)
	}
}

func TestS3RetriesServerErrors(t *testing.T) {
	f, st := newFakeS3(t)
	f.failNext = 2
	if err := st.Put(context.Background(), "k", []byte("x")); err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
}
