// Package agentfs is the guarded workspace AI agents act through. Every
// write is journalled in the vault before it happens, deletes are held for a
// human, and one agent's changes can be undone without touching anyone
// else's (ADR-0007).
package agentfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HarshiniTayi23/Ageis/internal/journal"
)

// MaxFileSize bounds what an agent may read or write in one call.
const MaxFileSize = 10 << 20

var (
	ErrOutside  = errors.New("agentfs: path is outside the workspace")
	ErrTooLarge = errors.New("agentfs: file too large")
)

// Workspace is one directory tree, exposed to one identity.
type Workspace struct {
	root  string
	agent string
	j     *journal.Journal
}

// Open returns a workspace rooted at dir acting as identity. For agents the
// identity is fixed by whoever starts the tool server, never by the agent.
func Open(dir, identity string, j *journal.Journal) (*Workspace, error) {
	if identity == "" {
		return nil, errors.New("agentfs: identity is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("agentfs: workspace: %w", err)
	}
	return &Workspace{root: abs, agent: identity, j: j}, nil
}

// Agent returns the workspace identity.
func (w *Workspace) Agent() string { return w.agent }

// resolve maps a workspace-relative path to an absolute one, refusing
// anything that escapes the root: "..", absolute paths, and symlinks that
// point outside.
func (w *Workspace) resolve(rel string) (abs, clean string, err error) {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", "", fmt.Errorf("%w: %q", ErrOutside, rel)
	}
	clean = filepath.ToSlash(filepath.Clean(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", "", fmt.Errorf("%w: %q", ErrOutside, rel)
	}
	abs = filepath.Join(w.root, filepath.FromSlash(clean))
	// Resolve symlinks on the deepest existing ancestor.
	probe := abs
	for {
		real, err := filepath.EvalSymlinks(probe)
		if err == nil {
			if real != w.root && !strings.HasPrefix(real, w.root+string(os.PathSeparator)) {
				return "", "", fmt.Errorf("%w: %q resolves outside", ErrOutside, rel)
			}
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	return abs, clean, nil
}

// state reads a file's current state; with keep, its content is stored as a
// before-image blob in the vault.
func (w *Workspace) state(ctx context.Context, abs string, keep bool) (journal.FileState, error) {
	fi, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return journal.FileState{}, nil
	}
	if err != nil {
		return journal.FileState{}, err
	}
	if !fi.Mode().IsRegular() {
		return journal.FileState{}, fmt.Errorf("agentfs: %s is not a regular file", filepath.Base(abs))
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return journal.FileState{}, err
	}
	st := journal.FileState{Exists: true, SHA256: sum(data), Size: int64(len(data))}
	if keep {
		if st.Blob, err = w.j.PutBlob(ctx, data); err != nil {
			return journal.FileState{}, err
		}
	}
	return st, nil
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// ReadFile returns a file's content. Reads are not journalled.
func (w *Workspace) ReadFile(rel string) ([]byte, error) {
	abs, _, err := w.resolve(rel)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if fi.Size() > MaxFileSize {
		return nil, ErrTooLarge
	}
	return os.ReadFile(abs)
}

// Entry is one item in a directory listing.
type Entry struct {
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Bytes int64  `json:"bytes,omitempty"`
}

// List returns a directory's entries ("." for the root).
func (w *Workspace) List(rel string) ([]Entry, error) {
	abs := w.root
	if rel != "" && rel != "." {
		var err error
		if abs, _, err = w.resolve(rel); err != nil {
			return nil, err
		}
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, d := range des {
		e := Entry{Name: d.Name(), Dir: d.IsDir()}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			e.Bytes = info.Size()
		}
		out = append(out, e)
	}
	return out, nil
}

// WriteFile creates or replaces a file. The event, including the previous
// content, is journalled BEFORE the file changes, so no change can happen
// unrecorded. (If the write then fails, undo sees a mismatch and reports a
// conflict instead of guessing.)
func (w *Workspace) WriteFile(ctx context.Context, rel string, data []byte) (*journal.Event, error) {
	if len(data) > MaxFileSize {
		return nil, ErrTooLarge
	}
	abs, clean, err := w.resolve(rel)
	if err != nil {
		return nil, err
	}
	before, err := w.state(ctx, abs, true)
	if err != nil {
		return nil, err
	}
	after := journal.FileState{Exists: true, SHA256: sum(data), Size: int64(len(data))}
	if before.Same(after) {
		return nil, nil // no-op: nothing to record
	}
	ev := &journal.Event{Agent: w.agent, Kind: journal.KindWrite, Path: clean, Before: before, After: after}
	if err := w.j.Append(ctx, ev); err != nil {
		return nil, fmt.Errorf("agentfs: journal unavailable, refusing to write: %w", err)
	}
	return ev, atomicWrite(abs, data)
}

// RequestDelete records a request to delete a file. Nothing is deleted
// until a human calls Approve.
func (w *Workspace) RequestDelete(ctx context.Context, rel string) (*journal.Event, error) {
	abs, clean, err := w.resolve(rel)
	if err != nil {
		return nil, err
	}
	before, err := w.state(ctx, abs, true)
	if err != nil {
		return nil, err
	}
	if !before.Exists {
		return nil, fmt.Errorf("agentfs: %s does not exist", clean)
	}
	ev := &journal.Event{Agent: w.agent, Kind: journal.KindDeleteRequested, Path: clean, Before: before}
	return ev, w.j.Append(ctx, ev)
}

// Approve executes a pending delete request, if the file is unchanged since
// the request was made. The workspace identity is recorded as the approver.
func (w *Workspace) Approve(ctx context.Context, requestID string) (*journal.Event, error) {
	req, err := w.pending(ctx, requestID)
	if err != nil {
		return nil, err
	}
	abs, _, err := w.resolve(req.Path)
	if err != nil {
		return nil, err
	}
	cur, err := w.state(ctx, abs, false)
	if err != nil {
		return nil, err
	}
	if !cur.Same(req.Before) {
		return nil, fmt.Errorf("agentfs: %s changed since the delete was requested; reject it and ask again", req.Path)
	}
	ev := &journal.Event{Agent: w.agent, Kind: journal.KindDeleteApproved, Path: req.Path,
		Before: req.Before, After: journal.FileState{}, Ref: req.ID}
	if err := w.j.Append(ctx, ev); err != nil {
		return nil, err
	}
	return ev, os.Remove(abs)
}

// Reject closes a pending delete request without deleting anything.
func (w *Workspace) Reject(ctx context.Context, requestID, reason string) (*journal.Event, error) {
	req, err := w.pending(ctx, requestID)
	if err != nil {
		return nil, err
	}
	ev := &journal.Event{Agent: w.agent, Kind: journal.KindDeleteRejected, Path: req.Path, Ref: req.ID, Note: reason}
	return ev, w.j.Append(ctx, ev)
}

func (w *Workspace) pending(ctx context.Context, id string) (*journal.Event, error) {
	st, err := w.j.Replay(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range st.Pending {
		if p.ID == id || (len(id) >= 8 && strings.HasPrefix(p.ID, id)) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("agentfs: no pending delete request %q", id)
}

// ------------------------------------------------------------------ undo ---

// UndoAction is one line of an undo plan.
type UndoAction struct {
	EventID string `json:"event_id"`
	Path    string `json:"path"`
	Action  string `json:"action"` // restore | remove | recreate | conflict | pending
	Reason  string `json:"reason,omitempty"`
}

// Undo reverts every change made by agent, newest first. A change is only
// reverted if the file still holds exactly what the agent left; otherwise
// someone changed it since, and it is reported as a conflict and left alone.
// With dryRun nothing is changed.
func (w *Workspace) Undo(ctx context.Context, agent string, dryRun bool) ([]UndoAction, error) {
	st, err := w.j.Replay(ctx)
	if err != nil {
		return nil, err
	}
	var targets []*journal.Event
	for _, e := range st.Events {
		if st.Undone[e.ID] || st.ActedFor(e) != agent {
			continue
		}
		if e.Kind == journal.KindWrite || e.Kind == journal.KindDeleteApproved {
			targets = append(targets, e)
		}
	}
	sort.Slice(targets, func(a, b int) bool { return targets[a].ID > targets[b].ID })

	var plan []UndoAction
	blocked := map[string]bool{} // a conflict on a path stops older reverts on it
	for _, e := range targets {
		act := UndoAction{EventID: e.ID, Path: e.Path}
		abs, _, err := w.resolve(e.Path)
		if err != nil {
			return nil, err
		}
		cur, err := w.state(ctx, abs, false)
		if err != nil {
			return nil, err
		}
		switch {
		case blocked[e.Path]:
			act.Action, act.Reason = "conflict", "a newer change to this file could not be undone"
		case !cur.Same(e.After):
			act.Action, act.Reason = "conflict", "file changed after the agent's edit (likely a human); left as is"
			blocked[e.Path] = true
		case !e.Before.Exists:
			act.Action = "remove"
		case e.Kind == journal.KindDeleteApproved:
			act.Action = "recreate"
		default:
			act.Action = "restore"
		}
		plan = append(plan, act)
		if dryRun || act.Action == "conflict" {
			continue
		}
		if err := w.revert(ctx, abs, cur, e); err != nil {
			return plan, fmt.Errorf("agentfs: undo %s: %w", e.Path, err)
		}
	}
	for _, p := range st.Pending {
		if p.Agent == agent {
			plan = append(plan, UndoAction{EventID: p.ID, Path: p.Path, Action: "pending",
				Reason: "delete was never executed; reject it with: aegis agent-reject " + p.ID})
		}
	}
	return plan, nil
}

func (w *Workspace) revert(ctx context.Context, abs string, cur journal.FileState, e *journal.Event) error {
	ev := &journal.Event{Agent: "aegis-undo", Kind: journal.KindUndo, Path: e.Path, Before: cur, After: e.Before, Ref: e.ID}
	if err := w.j.Append(ctx, ev); err != nil {
		return err
	}
	if !e.Before.Exists {
		return os.Remove(abs)
	}
	data, err := w.j.Blob(ctx, e.Before.Blob)
	if err != nil {
		return err
	}
	return atomicWrite(abs, data)
}

func atomicWrite(abs string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".aegis-tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil { //nolint:gosec // G302: workspace files are ordinary user files
		_ = os.Remove(name)
		return err
	}
	return os.Rename(name, abs)
}
