// Package journal is the second ingest path into the vault: an append-only,
// event-sourced record of every change an AI agent makes, so one agent's
// actions can be inspected and rolled back without touching human work.
//
// The vault is write-once, so nothing is ever updated in place. Even
// "is this delete still pending?" is derived by replaying events
// (see ADR-0007). History therefore cannot be rewritten, by the agent or by
// anyone else.
//
// Layout in the object store:
//
//	journal/<unixnano>-<rand>     sealed JSON Event
//	journal-blobs/<keyed hash>    sealed file content (before-images), deduplicated
package journal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HarshiniTayi23/Ageis/internal/store"
)

// Kind is the type of a journal event.
type Kind string

const (
	KindWrite           Kind = "write"            // agent created or changed a file
	KindDeleteRequested Kind = "delete-requested" // agent asked to delete; held for a human
	KindDeleteApproved  Kind = "delete-approved"  // a human approved; the file was deleted
	KindDeleteRejected  Kind = "delete-rejected"  // a human rejected; nothing happened
	KindUndo            Kind = "undo"             // an earlier event was reverted
)

// FileState captures a file at one moment.
type FileState struct {
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Blob   string `json:"blob,omitempty"` // journal-blobs key holding the content
}

// Same reports whether two states describe the same content.
func (f FileState) Same(o FileState) bool {
	return f.Exists == o.Exists && (!f.Exists || f.SHA256 == o.SHA256)
}

// Event is one immutable journal entry.
type Event struct {
	ID     string    `json:"id"`
	Time   time.Time `json:"time"`
	Agent  string    `json:"agent"` // who acted: an agent name, "human:<name>", or "aegis-undo"
	Kind   Kind      `json:"kind"`
	Path   string    `json:"path"` // workspace-relative, slash-separated
	Before FileState `json:"before"`
	After  FileState `json:"after"`
	Ref    string    `json:"ref,omitempty"` // approved/rejected: the request; undo: the reverted event
	Note   string    `json:"note,omitempty"`
}

// Sealer is the subset of seal.Keyring the journal needs.
type Sealer interface {
	Seal(label string, plain []byte) ([]byte, error)
	Open(label string, blob []byte) ([]byte, error)
	ID(plain []byte) string
}

// Journal reads and appends events.
type Journal struct {
	st  store.ObjectStore
	s   Sealer
	Now func() time.Time
}

// New returns a journal over the vault.
func New(st store.ObjectStore, s Sealer) *Journal {
	return &Journal{st: st, s: s, Now: time.Now}
}

// PutBlob stores content and returns its key. Identical content is stored once.
func (j *Journal) PutBlob(ctx context.Context, data []byte) (string, error) {
	key := "journal-blobs/" + j.s.ID(data)
	if ok, err := j.st.Exists(ctx, key); err != nil {
		return "", err
	} else if ok {
		return key, nil
	}
	sealed, err := j.s.Seal(key, data)
	if err != nil {
		return "", err
	}
	if err := j.st.Put(ctx, key, sealed); err != nil && !errors.Is(err, store.ErrExists) {
		return "", err
	}
	return key, nil
}

// Blob returns stored content.
func (j *Journal) Blob(ctx context.Context, key string) ([]byte, error) {
	if !strings.HasPrefix(key, "journal-blobs/") {
		return nil, fmt.Errorf("journal: not a blob key: %q", key)
	}
	sealed, err := j.st.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	data, err := j.s.Open(key, sealed)
	if err != nil {
		return nil, err
	}
	if "journal-blobs/"+j.s.ID(data) != key {
		return nil, fmt.Errorf("journal: blob %s does not match its address", key)
	}
	return data, nil
}

// Append assigns e an ID and time and writes it to the vault.
func (j *Journal) Append(ctx context.Context, e *Event) error {
	e.Time = j.Now().UTC()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	e.ID = fmt.Sprintf("%019d-%s", e.Time.UnixNano(), hex.EncodeToString(b))
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	key := "journal/" + e.ID
	sealed, err := j.s.Seal(key, body)
	if err != nil {
		return err
	}
	return j.st.Put(ctx, key, sealed)
}

// Events returns every event, oldest first.
func (j *Journal) Events(ctx context.Context) ([]*Event, error) {
	keys, err := j.st.List(ctx, "journal/")
	if err != nil {
		return nil, err
	}
	out := make([]*Event, 0, len(keys))
	for _, k := range keys {
		sealed, err := j.st.Get(ctx, k)
		if err != nil {
			return nil, err
		}
		body, err := j.s.Open(k, sealed)
		if err != nil {
			return nil, fmt.Errorf("journal: %s: %w", k, err)
		}
		var e Event
		if err := json.Unmarshal(body, &e); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out, nil
}

// State is the journal replayed into current facts.
type State struct {
	Events  []*Event
	ByID    map[string]*Event
	Pending []*Event        // delete requests with no decision yet
	Undone  map[string]bool // event IDs already reverted
	Decided map[string]*Event
}

// Replay derives current state from the event log.
func (j *Journal) Replay(ctx context.Context) (*State, error) {
	evs, err := j.Events(ctx)
	if err != nil {
		return nil, err
	}
	s := &State{Events: evs, ByID: map[string]*Event{}, Undone: map[string]bool{}, Decided: map[string]*Event{}}
	for _, e := range evs {
		s.ByID[e.ID] = e
		switch e.Kind {
		case KindDeleteApproved, KindDeleteRejected:
			s.Decided[e.Ref] = e
		case KindUndo:
			s.Undone[e.Ref] = true
		}
	}
	for _, e := range evs {
		if e.Kind == KindDeleteRequested && s.Decided[e.ID] == nil {
			s.Pending = append(s.Pending, e)
		}
	}
	return s, nil
}

// ActedFor returns the agent responsible for an event's effect. For an
// approved delete that is the agent who requested it, not the approver.
func (s *State) ActedFor(e *Event) string {
	if e.Kind == KindDeleteApproved {
		if req := s.ByID[e.Ref]; req != nil {
			return req.Agent
		}
	}
	return e.Agent
}
