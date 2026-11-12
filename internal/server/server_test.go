package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HarshiniTayi23/Ageis/internal/repo"
	"github.com/HarshiniTayi23/Ageis/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.NewLocalWORM(filepath.Join(t.TempDir(), "vault"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r, err := repo.Init(context.Background(), st, "pw")
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(filepath.Join(src, "f"+string(rune('a'+i))+".csv"), []byte(strings.Repeat("a,b,c\n", 200)), 0o600)
	}
	ts := httptest.NewServer(New(r, src, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func call(t *testing.T, method, url string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestAPIBackupDrillAndMetrics(t *testing.T) {
	ts := newTestServer(t)
	if code, _ := call(t, "GET", ts.URL+"/healthz"); code != 200 {
		t.Fatalf("healthz %d", code)
	}
	code, body := call(t, "POST", ts.URL+"/api/v1/backups")
	if code != http.StatusCreated {
		t.Fatalf("backup %d %s", code, body)
	}
	var sum struct {
		ID    string `json:"id"`
		Files int    `json:"files"`
	}
	_ = json.Unmarshal([]byte(body), &sum)
	if sum.Files != 5 {
		t.Fatalf("backup summary %s", body)
	}
	if code, body := call(t, "GET", ts.URL+"/api/v1/snapshots/"+sum.ID); code != 200 || !strings.Contains(body, sum.ID) {
		t.Fatalf("get snapshot %d", code)
	}
	if code, body := call(t, "POST", ts.URL+"/api/v1/drills"); code != http.StatusCreated || !strings.Contains(body, `"passed": true`) {
		t.Fatalf("drill %d %s", code, body)
	}

	_, m := call(t, "GET", ts.URL+"/metrics")
	for _, want := range []string{
		`aegis_backups_total{result="ok"} 1`,
		`aegis_last_drill_passed 1`,
		`aegis_backup_duration_seconds_count 1`,
		`aegis_http_requests_total{method="POST",route="/api/v1/backups",code="201"} 1`,
		// the snapshot ID must NOT become a label value: route patterns keep cardinality bounded
		`route="/api/v1/snapshots/{id}"`,
	} {
		if !strings.Contains(m, want) {
			t.Errorf("metrics missing %q\n%s", want, m)
		}
	}
	if strings.Contains(m, sum.ID) {
		t.Error("snapshot id leaked into a metric label")
	}
}
