package agentfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HarshiniTayi23/Ageis/internal/journal"
	"github.com/HarshiniTayi23/Ageis/internal/repo"
	"github.com/HarshiniTayi23/Ageis/internal/store"
)

func setup(t *testing.T) (*journal.Journal, string) {
	t.Helper()
	st, err := store.NewLocalWORM(filepath.Join(t.TempDir(), "vault"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r, err := repo.Init(context.Background(), st, "pw")
	if err != nil {
		t.Fatal(err)
	}
	return journal.New(r.Vault()), t.TempDir()
}

func agentAndHuman(t *testing.T) (*Workspace, *Workspace, string) {
	j, dir := setup(t)
	agent, err := Open(dir, "build-bot", j)
	if err != nil {
		t.Fatal(err)
	}
	human, _ := Open(dir, "human:arjun", j)
	return agent, human, dir
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func TestPathsCannotEscapeWorkspace(t *testing.T) {
	ctx := context.Background()
	w, _, dir := agentAndHuman(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../x", "/etc/passwd", "a/../../x", "link/x", ".."} {
		if _, err := w.WriteFile(ctx, p, []byte("pwned")); !errors.Is(err, ErrOutside) {
			t.Errorf("WriteFile(%q) = %v, want ErrOutside", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "x")); err == nil {
		t.Fatal("wrote through a symlink outside the workspace")
	}
}

func TestDeleteIsHeldUntilApproved(t *testing.T) {
	ctx := context.Background()
	agent, human, dir := agentAndHuman(t)
	_ = os.WriteFile(filepath.Join(dir, "customers.db"), []byte("rows"), 0o644)

	req, err := agent.RequestDelete(ctx, "customers.db")
	if err != nil {
		t.Fatal(err)
	}
	if read(t, dir, "customers.db") != "rows" {
		t.Fatal("file deleted before approval")
	}
	if _, err := human.Approve(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if read(t, dir, "customers.db") != "<missing>" {
		t.Fatal("approved delete did not happen")
	}
	// The agent's delete is undoable even after approval.
	plan, err := human.Undo(ctx, "build-bot", false)
	if err != nil || len(plan) != 1 || plan[0].Action != "recreate" {
		t.Fatalf("plan %+v, %v", plan, err)
	}
	if read(t, dir, "customers.db") != "rows" {
		t.Fatal("undo did not recreate the deleted file")
	}
}

func TestApproveRefusesIfFileChangedSinceRequest(t *testing.T) {
	ctx := context.Background()
	agent, human, dir := agentAndHuman(t)
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("v1"), 0o644)
	req, _ := agent.RequestDelete(ctx, "a.txt")
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("v2 - important new work"), 0o644)
	if _, err := human.Approve(ctx, req.ID); err == nil {
		t.Fatal("approved a delete of a file that changed after the request")
	}
}

// The core promise: undo one agent, keep everyone else's work.
func TestUndoRevertsOnlyTheAgentAndRespectsHumanEdits(t *testing.T) {
	ctx := context.Background()
	agent, human, dir := agentAndHuman(t)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("replicas: 3\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "notes.md"), []byte("v1\n"), 0o644)

	// Agent: overwrite config, create a file, edit notes.
	_, _ = agent.WriteFile(ctx, "config.yaml", []byte("replicas: 0  # oops\n"))
	_, _ = agent.WriteFile(ctx, "scratch/new.txt", []byte("tmp"))
	_, _ = agent.WriteFile(ctx, "notes.md", []byte("v2 by agent\n"))
	// Human then edits notes.md outside the tool server.
	_ = os.WriteFile(filepath.Join(dir, "notes.md"), []byte("v3 by human\n"), 0o644)
	// Human also writes through the workspace under their own identity.
	_, _ = human.WriteFile(ctx, "human.txt", []byte("mine"))

	dry, err := human.Undo(ctx, "build-bot", true)
	if err != nil {
		t.Fatal(err)
	}
	if read(t, dir, "config.yaml") != "replicas: 0  # oops\n" {
		t.Fatal("dry run changed files")
	}
	actions := map[string]string{}
	for _, a := range dry {
		actions[a.Path] = a.Action
	}
	want := map[string]string{"config.yaml": "restore", "scratch/new.txt": "remove", "notes.md": "conflict"}
	for p, a := range want {
		if actions[p] != a {
			t.Errorf("%s: action %q, want %q (plan %+v)", p, actions[p], a, dry)
		}
	}

	if _, err := human.Undo(ctx, "build-bot", false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "config.yaml"); got != "replicas: 3\n" {
		t.Errorf("config.yaml = %q", got)
	}
	if got := read(t, dir, "scratch/new.txt"); got != "<missing>" {
		t.Errorf("agent-created file not removed: %q", got)
	}
	if got := read(t, dir, "notes.md"); got != "v3 by human\n" {
		t.Errorf("human edit lost: notes.md = %q", got)
	}
	if got := read(t, dir, "human.txt"); got != "mine" {
		t.Errorf("other identity's file touched: %q", got)
	}

	// Undo is recorded and not repeated.
	again, _ := human.Undo(ctx, "build-bot", true)
	for _, a := range again {
		if a.Action != "conflict" {
			t.Errorf("already-undone event planned again: %+v", a)
		}
	}
}

func TestMultipleAgentEditsToOneFileUndoInOrder(t *testing.T) {
	ctx := context.Background()
	agent, human, dir := agentAndHuman(t)
	_ = os.WriteFile(filepath.Join(dir, "f"), []byte("original"), 0o644)
	for _, v := range []string{"a", "b", "c"} {
		_, _ = agent.WriteFile(ctx, "f", []byte(v))
	}
	if _, err := human.Undo(ctx, "build-bot", false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "f"); got != "original" {
		t.Fatalf("f = %q, want original", got)
	}
}

func TestJournalIsInTheImmutableVault(t *testing.T) {
	ctx := context.Background()
	j, dir := setup(t)
	agent, _ := Open(dir, "build-bot", j)
	ev, _ := agent.WriteFile(ctx, "x", []byte("1"))
	evs, err := j.Events(ctx)
	if err != nil || len(evs) != 1 || evs[0].ID != ev.ID || evs[0].Agent != "build-bot" {
		t.Fatalf("events: %v %+v", err, evs)
	}
}
