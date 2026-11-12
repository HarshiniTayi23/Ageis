package agentfs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/HarshiniTayi23/Ageis/internal/mcp"
)

// Tools returns the MCP tools an agent gets for this workspace. Tool
// descriptions tell the agent the rules up front, so a well-behaved agent
// never needs to discover them by failing.
func (w *Workspace) Tools() []mcp.Tool {
	pathArg := map[string]any{"type": "string", "description": "path relative to the workspace root"}
	schema := func(required []string, props map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	type pathArgs struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	parse := func(raw json.RawMessage) (pathArgs, error) {
		var a pathArgs
		if err := json.Unmarshal(raw, &a); err != nil {
			return a, fmt.Errorf("invalid arguments: %w", err)
		}
		return a, nil
	}
	return []mcp.Tool{
		{
			Name:        "read_file",
			Description: "Read a UTF-8 text file from the workspace.",
			InputSchema: schema([]string{"path"}, map[string]any{"path": pathArg}),
			Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
				a, err := parse(raw)
				if err != nil {
					return "", err
				}
				b, err := w.ReadFile(a.Path)
				return string(b), err
			},
		},
		{
			Name:        "list_directory",
			Description: "List files and directories. Use \".\" for the workspace root.",
			InputSchema: schema([]string{"path"}, map[string]any{"path": pathArg}),
			Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
				a, err := parse(raw)
				if err != nil {
					return "", err
				}
				es, err := w.List(a.Path)
				if err != nil {
					return "", err
				}
				var b strings.Builder
				for _, e := range es {
					if e.Dir {
						fmt.Fprintf(&b, "%s/\n", e.Name)
					} else {
						fmt.Fprintf(&b, "%s (%d bytes)\n", e.Name, e.Bytes)
					}
				}
				return b.String(), nil
			},
		},
		{
			Name: "write_file",
			Description: "Create or overwrite a text file. Every write is recorded with its previous " +
				"content so it can be undone.",
			InputSchema: schema([]string{"path", "content"}, map[string]any{
				"path": pathArg, "content": map[string]any{"type": "string"},
			}),
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				a, err := parse(raw)
				if err != nil {
					return "", err
				}
				ev, err := w.WriteFile(ctx, a.Path, []byte(a.Content))
				if err != nil {
					return "", err
				}
				if ev == nil {
					return "unchanged: " + a.Path, nil
				}
				return fmt.Sprintf("wrote %s (%d bytes, change %s)", ev.Path, ev.After.Size, ev.ID), nil
			},
		},
		{
			Name: "delete_file",
			Description: "Request deletion of a file. Deletions are NOT immediate: they are held until a " +
				"human approves them. Continue with other work; do not retry.",
			InputSchema: schema([]string{"path"}, map[string]any{"path": pathArg}),
			Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
				a, err := parse(raw)
				if err != nil {
					return "", err
				}
				ev, err := w.RequestDelete(ctx, a.Path)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("delete of %s is pending human approval (request %s). "+
					"The file has not been deleted.", ev.Path, ev.ID), nil
			},
		},
	}
}
