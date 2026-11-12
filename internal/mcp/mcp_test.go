package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func run(t *testing.T, s *Server, lines ...string) []map[string]any {
	t.Helper()
	var out strings.Builder
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var resps []map[string]any
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("server wrote a non-JSON line: %q", sc.Text())
		}
		resps = append(resps, m)
	}
	return resps
}

func testServer() *Server {
	return &Server{Name: "test", Version: "0", Tools: []Tool{
		{Name: "echo", InputSchema: map[string]any{"type": "object"},
			Handler: func(_ context.Context, a json.RawMessage) (string, error) { return string(a), nil }},
		{Name: "fail", InputSchema: map[string]any{"type": "object"},
			Handler: func(context.Context, json.RawMessage) (string, error) { return "", errors.New("nope") }},
	}}
}

func TestHandshakeListAndCall(t *testing.T) {
	r := run(t, testServer(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":"three","method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"fail","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"bogus"}`,
	)
	if len(r) != 5 { // the notification gets no reply
		t.Fatalf("got %d responses: %v", len(r), r)
	}
	if v := r[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-03-26" {
		t.Errorf("negotiated version %v", v)
	}
	if n := len(r[1]["result"].(map[string]any)["tools"].([]any)); n != 2 {
		t.Errorf("tools/list returned %d tools", n)
	}
	if r[2]["id"] != "three" {
		t.Errorf("string id not echoed: %v", r[2]["id"])
	}
	call := r[2]["result"].(map[string]any)
	if call["isError"] != false || !strings.Contains(call["content"].([]any)[0].(map[string]any)["text"].(string), `"x":1`) {
		t.Errorf("echo result %v", call)
	}
	if r[3]["result"].(map[string]any)["isError"] != true {
		t.Errorf("tool error not flagged: %v", r[3])
	}
	if r[4]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Errorf("unknown method: %v", r[4])
	}
}

func TestMalformedLineGetsParseError(t *testing.T) {
	r := run(t, testServer(), `{not json`)
	if len(r) != 1 || r[0]["error"].(map[string]any)["code"].(float64) != -32700 {
		t.Fatalf("got %v", r)
	}
}
