package server

// Prometheus metrics in the plain-text exposition format, on the standard
// library (no client dependency). Exposed at GET /metrics.
//
// The metrics are chosen around the two personas:
//   - Ravi (on call): is backup running, is it fast, did the last one look
//     like an attack?
//   - Meera (CISO): how many suspicious snapshots, when did we last prove a
//     restore works?

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/HarshiniTayi23/Ageis/internal/repo"
)

type metrics struct {
	mu                  sync.Mutex
	backups             map[string]float64 // result -> count
	backupBytes         float64
	uploadedBytes       float64
	suspicious          float64
	lastBackupTime      float64
	lastBackupSeconds   float64
	lastAnomalyScore    float64
	lastDrillTime       float64
	lastDrillPassed     float64
	requests            map[string]float64 // "method path code" -> count
	durationBuckets     []float64
	backupDurationHist  []float64 // cumulative counts per bucket
	backupDurationSum   float64
	backupDurationCount float64
}

func newMetrics() *metrics {
	b := []float64{0.1, 0.5, 1, 5, 15, 60, 300, 1800}
	return &metrics{
		backups: map[string]float64{}, requests: map[string]float64{},
		durationBuckets: b, backupDurationHist: make([]float64, len(b)),
	}
}

func (m *metrics) observeBackup(sn *repo.Snapshot, err error, took time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.backups["error"]++
		return
	}
	m.backups["ok"]++
	m.backupBytes += float64(sn.Stats.Bytes)
	m.uploadedBytes += float64(sn.Stats.NewBytes)
	m.lastBackupTime = float64(sn.Time.Unix())
	m.lastBackupSeconds = took.Seconds()
	m.lastAnomalyScore = sn.Verdict.Score
	if sn.Verdict.Suspicious {
		m.suspicious++
	}
	m.backupDurationSum += took.Seconds()
	m.backupDurationCount++
	for i, le := range m.durationBuckets {
		if took.Seconds() <= le {
			m.backupDurationHist[i]++
		}
	}
}

func (m *metrics) observeDrill(d *repo.DrillReport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastDrillTime = float64(d.Time.Unix())
	m.lastDrillPassed = 0
	if d.Passed {
		m.lastDrillPassed = 1
	}
}

func (m *metrics) observeRequest(method, pattern string, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[fmt.Sprintf("%s %s %d", method, pattern, code)]++
}

func (m *metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	metric := func(name, typ, help string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
	}
	metric("aegis_backups_total", "counter", "Backups run, by result.")
	for _, res := range []string{"ok", "error"} {
		fmt.Fprintf(&b, "aegis_backups_total{result=%q} %g\n", res, m.backups[res])
	}
	metric("aegis_backup_bytes_total", "counter", "Logical bytes covered by backups.")
	fmt.Fprintf(&b, "aegis_backup_bytes_total %g\n", m.backupBytes)
	metric("aegis_backup_uploaded_bytes_total", "counter", "Bytes actually uploaded after deduplication.")
	fmt.Fprintf(&b, "aegis_backup_uploaded_bytes_total %g\n", m.uploadedBytes)
	metric("aegis_suspicious_snapshots_total", "counter", "Snapshots flagged with ransomware indicators.")
	fmt.Fprintf(&b, "aegis_suspicious_snapshots_total %g\n", m.suspicious)
	metric("aegis_last_anomaly_score", "gauge", "Anomaly score of the most recent snapshot (0..1).")
	fmt.Fprintf(&b, "aegis_last_anomaly_score %g\n", m.lastAnomalyScore)
	metric("aegis_last_backup_timestamp_seconds", "gauge", "Unix time of the last successful backup.")
	fmt.Fprintf(&b, "aegis_last_backup_timestamp_seconds %g\n", m.lastBackupTime)
	metric("aegis_last_drill_timestamp_seconds", "gauge", "Unix time of the last recovery drill.")
	fmt.Fprintf(&b, "aegis_last_drill_timestamp_seconds %g\n", m.lastDrillTime)
	metric("aegis_last_drill_passed", "gauge", "1 if the last recovery drill passed.")
	fmt.Fprintf(&b, "aegis_last_drill_passed %g\n", m.lastDrillPassed)
	metric("aegis_backup_duration_seconds", "histogram", "Backup duration.")
	for i, le := range m.durationBuckets {
		fmt.Fprintf(&b, "aegis_backup_duration_seconds_bucket{le=\"%g\"} %g\n", le, m.backupDurationHist[i])
	}
	fmt.Fprintf(&b, "aegis_backup_duration_seconds_bucket{le=\"+Inf\"} %g\n", m.backupDurationCount)
	fmt.Fprintf(&b, "aegis_backup_duration_seconds_sum %g\naegis_backup_duration_seconds_count %g\n", m.backupDurationSum, m.backupDurationCount)
	metric("aegis_http_requests_total", "counter", "HTTP requests by method, route and status.")
	keys := make([]string, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := strings.SplitN(k, " ", 3)
		fmt.Fprintf(&b, "aegis_http_requests_total{method=%q,route=%q,code=%q} %g\n", p[0], p[1], p[2], m.requests[k])
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(b.String()))
}

// statusRecorder captures the status code for request metrics.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}
