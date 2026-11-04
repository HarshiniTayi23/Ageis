// Package server exposes the backup engine as a small HTTP/JSON service so it
// can run as a containerised microservice (see deploy/k8s).
//
//	GET  /healthz                     liveness
//	GET  /readyz                      readiness (repository reachable)
//	GET  /api/v1/snapshots            list snapshots with verdicts
//	GET  /api/v1/snapshots/{id}       one snapshot (file list included)
//	GET  /api/v1/recovery-point       latest clean snapshot
//	POST /api/v1/backups              run a backup of the configured source
//	POST /api/v1/verify?deep=true     integrity check
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/HarshiniTayi23/Ageis/internal/repo"
)

type Server struct {
	repo   *repo.Repo
	source string
	log    *slog.Logger
	mu     sync.Mutex // one backup at a time
}

func New(r *repo.Repo, source string, log *slog.Logger) *Server {
	return &Server{repo: r, source: source, log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /api/v1/snapshots", s.list)
	mux.HandleFunc("GET /api/v1/snapshots/{id}", s.get)
	mux.HandleFunc("GET /api/v1/recovery-point", s.recoveryPoint)
	mux.HandleFunc("POST /api/v1/backups", s.backup)
	mux.HandleFunc("POST /api/v1/verify", s.verify)
	return s.logging(mux)
}

type summary struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Files      int       `json:"files"`
	Bytes      int64     `json:"bytes"`
	NewBytes   int64     `json:"new_bytes"`
	Score      float64   `json:"anomaly_score"`
	Suspicious bool      `json:"suspicious"`
	Reasons    []string  `json:"reasons,omitempty"`
}

func summarize(sn *repo.Snapshot) summary {
	return summary{sn.ID, sn.Time, sn.Stats.Files, sn.Stats.Bytes, sn.Stats.NewBytes,
		sn.Verdict.Score, sn.Verdict.Suspicious, sn.Verdict.Reasons}
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if _, err := s.repo.Snapshots(ctx); err != nil && !errors.Is(err, repo.ErrNoSnapshots) {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	all, err := s.repo.Snapshots(r.Context())
	if errors.Is(err, repo.ErrNoSnapshots) {
		writeJSON(w, http.StatusOK, []summary{})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]summary, len(all))
	for i, sn := range all {
		out[i] = summarize(sn)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	sn, err := s.repo.Snapshot(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, sn)
}

func (s *Server) recoveryPoint(w http.ResponseWriter, r *http.Request) {
	sn, err := s.repo.LatestClean(r.Context())
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, summarize(sn))
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	if !s.mu.TryLock() {
		writeErr(w, http.StatusConflict, errors.New("a backup is already running"))
		return
	}
	defer s.mu.Unlock()
	sn, err := s.repo.Backup(r.Context(), s.source)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if sn.Verdict.Suspicious {
		s.log.Warn("ransomware indicators in new snapshot", "id", sn.ID, "score", sn.Verdict.Score, "reasons", sn.Verdict.Reasons)
	}
	writeJSON(w, http.StatusCreated, summarize(sn))
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	rep, err := s.repo.Verify(r.Context(), r.URL.Query().Get("deep") == "true")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	status := http.StatusOK
	if len(rep.Problems) > 0 {
		status = http.StatusConflict
	}
	writeJSON(w, status, rep)
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v) // headers already sent; nothing useful to do on error
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
