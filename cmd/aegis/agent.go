package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/HarshiniTayi23/Ageis/internal/agentfs"
	"github.com/HarshiniTayi23/Ageis/internal/journal"
	"github.com/HarshiniTayi23/Ageis/internal/mcp"
	"github.com/HarshiniTayi23/Ageis/internal/repo"
)

// agentOpts are the flags used by the agent-* commands.
type agentOpts struct {
	agent     string // agent-serve: identity of the agent; agent-log/undo: filter
	workspace string
	as        string // identity of the human running approve/reject/undo
	dryRun    bool
	json      bool
}

// runAgent handles agent-* commands. It returns handled=false for anything else.
//
//	aegis agent-serve   --agent NAME --workspace DIR     MCP tool server on stdio
//	aegis agent-log     [--agent NAME]                    journal timeline
//	aegis agent-pending                                   deletes awaiting approval
//	aegis agent-approve --workspace DIR ID
//	aegis agent-reject  --workspace DIR ID
//	aegis agent-undo    --workspace DIR --agent NAME [--dry-run]
func runAgent(ctx context.Context, cmd string, r *repo.Repo, o agentOpts, args []string) (bool, error) {
	if !strings.HasPrefix(cmd, "agent-") {
		return false, nil
	}
	j := journal.New(r.Vault())
	open := func(identity string) (*agentfs.Workspace, error) {
		if o.workspace == "" {
			return nil, errors.New("--workspace (or AEGIS_WORKSPACE) is required")
		}
		return agentfs.Open(o.workspace, identity, j)
	}

	switch cmd {
	case "agent-serve":
		if o.agent == "" {
			return true, errors.New("--agent is required: the identity is fixed at launch, never chosen by the agent")
		}
		w, err := open(o.agent)
		if err != nil {
			return true, err
		}
		fmt.Fprintf(os.Stderr, "aegis: serving workspace %s to agent %q over MCP (stdio)\n", o.workspace, o.agent)
		s := &mcp.Server{Name: "aegis-workspace", Version: "0.2.0", Tools: w.Tools()}
		return true, s.Serve(ctx, os.Stdin, os.Stdout)

	case "agent-log":
		st, err := j.Replay(ctx)
		if err != nil {
			return true, err
		}
		var evs []*journal.Event
		for _, e := range st.Events {
			if o.agent == "" || st.ActedFor(e) == o.agent || e.Agent == o.agent {
				evs = append(evs, e)
			}
		}
		if o.json {
			return true, printJSON(evs)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TIME\tWHO\tACTION\tPATH\tCHANGE\tID")
		for _, e := range evs {
			action := string(e.Kind)
			if st.Undone[e.ID] {
				action += " (undone)"
			}
			if e.Kind == journal.KindDeleteRequested && st.Decided[e.ID] == nil {
				action += " (PENDING)"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Time.Format(time.TimeOnly), e.Agent, action,
				e.Path, describe(e), e.ID)
		}
		return true, tw.Flush()

	case "agent-pending":
		st, err := j.Replay(ctx)
		if err != nil {
			return true, err
		}
		if len(st.Pending) == 0 {
			fmt.Println("no pending deletes")
			return true, nil
		}
		for _, p := range st.Pending {
			fmt.Printf("%s  %s wants to delete %s (%d bytes)\n", p.ID, p.Agent, p.Path, p.Before.Size)
		}
		return true, nil

	case "agent-approve", "agent-reject":
		if len(args) != 1 {
			return true, fmt.Errorf("usage: aegis %s --workspace DIR REQUEST_ID", cmd)
		}
		w, err := open(o.as)
		if err != nil {
			return true, err
		}
		var ev *journal.Event
		if cmd == "agent-approve" {
			ev, err = w.Approve(ctx, args[0])
		} else {
			ev, err = w.Reject(ctx, args[0], "rejected via CLI")
		}
		if err != nil {
			return true, err
		}
		fmt.Printf("%s: %s (%s by %s)\n", ev.Kind, ev.Path, ev.ID, ev.Agent)
		return true, nil

	case "agent-undo":
		if o.agent == "" {
			return true, errors.New("--agent is required")
		}
		w, err := open(o.as)
		if err != nil {
			return true, err
		}
		plan, err := w.Undo(ctx, o.agent, o.dryRun)
		if o.json && err == nil {
			return true, printJSON(plan)
		}
		verb := "undone"
		if o.dryRun {
			verb = "would undo (dry run, nothing changed)"
		}
		changes, conflicts := 0, 0
		for _, a := range plan {
			line := fmt.Sprintf("  %-9s %s", a.Action, a.Path)
			if a.Reason != "" {
				line += "  - " + a.Reason
			}
			switch a.Action {
			case "conflict":
				conflicts++
			case "restore", "remove", "recreate":
				changes++
			}
			fmt.Println(line)
		}
		fmt.Printf("%s %d change(s) by %q; %d conflict(s) left for a human\n", verb, changes, o.agent, conflicts)
		return true, err
	}
	return true, fmt.Errorf("unknown command %q", cmd)
}

func describe(e *journal.Event) string {
	switch {
	case e.Kind == journal.KindWrite && !e.Before.Exists:
		return fmt.Sprintf("created, %dB", e.After.Size)
	case e.Kind == journal.KindWrite:
		return fmt.Sprintf("%dB -> %dB", e.Before.Size, e.After.Size)
	case e.Kind == journal.KindUndo:
		return "reverts " + short(e.Ref)
	case e.Kind == journal.KindDeleteApproved || e.Kind == journal.KindDeleteRejected:
		return "for request " + short(e.Ref)
	}
	return fmt.Sprintf("%dB", e.Before.Size)
}

func short(id string) string {
	if len(id) > 12 {
		return id[len(id)-8:]
	}
	return id
}
