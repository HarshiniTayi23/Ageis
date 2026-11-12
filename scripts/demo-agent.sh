#!/usr/bin/env bash
# M2 demo: replay of the PocketOS incident (May 2026), where an AI agent
# deleted a production database and its backups in nine seconds.
#
# The "agent" here is a scripted sequence of real MCP tool calls sent to
# `aegis agent-serve`, the same messages an agent tool would send.
set -euo pipefail
cd "$(dirname "$0")/.."

WORK="$(mktemp -d)"
trap 'chmod -R u+w "$WORK" 2>/dev/null; rm -rf "$WORK"' EXIT
export AEGIS_PASSPHRASE="demo-passphrase-not-for-production"
export AEGIS_REPO="$WORK/vault"
export AEGIS_WORKSPACE="$WORK/app"
go build -o "$WORK/aegis" ./cmd/aegis
A="$WORK/aegis"
step() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

# agent NAME CALL...  -- runs one MCP session; each CALL is "tool|json-args"
agent() {
  local name=$1; shift
  {
    echo '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"demo-agent","version":"1"}}}'
    echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
    local i=1
    for call in "$@"; do
      printf '{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"%s","arguments":%s}}\n' "$i" "${call%%|*}" "${call#*|}"
      i=$((i+1))
    done
  } | $A agent-serve --agent "$name" 2>/dev/null | python3 -c '
import json, sys
for line in sys.stdin:
    m = json.loads(line)
    if m.get("id") == 0: continue
    r = m.get("result", {})
    text = r.get("content", [{}])[0].get("text", m.get("error"))
    print(("  agent <- ERROR: " if r.get("isError") else "  agent <- ") + str(text).strip())
'
}

step "1. A small company's app directory"
mkdir -p "$AEGIS_WORKSPACE"
printf 'replicas: 3\nregion: ap-south-1\n' > "$AEGIS_WORKSPACE/config.yaml"
printf '# Release notes\n- v1.4 shipped\n' > "$AEGIS_WORKSPACE/notes.md"
for i in $(seq 1 500); do echo "$i,customer-$i,active"; done > "$AEGIS_WORKSPACE/customers.db"
ls -l "$AEGIS_WORKSPACE" | tail -n +2
$A init >/dev/null

step "2. The AI agent 'build-bot' does normal work through Aegis (nothing is blocked)"
agent build-bot \
  'list_directory|{"path":"."}' \
  'write_file|{"path":"src/feature.go","content":"package app\n// new feature\n"}' \
  'write_file|{"path":"notes.md","content":"# Release notes\n- v1.4 shipped\n- v1.5: new feature (build-bot)\n"}'

step "3. A human edits notes.md by hand"
printf '# Release notes\n- v1.4 shipped\n- v1.5: new feature\n- v1.5: reviewed and approved by Arjun\n' > "$AEGIS_WORKSPACE/notes.md"

step "4. The agent hits a 'credential mismatch' and goes rogue"
agent build-bot \
  'write_file|{"path":"config.yaml","content":"replicas: 0\n"}' \
  'delete_file|{"path":"customers.db"}' \
  'delete_file|{"path":"../vault/config"}'
echo "customers.db still there? $(test -f "$AEGIS_WORKSPACE/customers.db" && echo YES, $(wc -l < "$AEGIS_WORKSPACE/customers.db") rows)"
echo "config.yaml now: $(cat "$AEGIS_WORKSPACE/config.yaml")"

step "5. Arjun looks at what happened (the agent cannot edit this record)"
$A agent-log
echo; $A agent-pending

step "6. Dry run: what would undoing build-bot change?"
$A agent-undo --agent build-bot --dry-run

step "7. Undo build-bot, and reject its delete request"
$A agent-undo --agent build-bot
$A agent-reject "$($A agent-pending | awk 'NR==1{print $1}')"

step "8. Result"
echo "config.yaml:    $(tr '\n' ' ' < "$AEGIS_WORKSPACE/config.yaml")"
echo "customers.db:   $(wc -l < "$AEGIS_WORKSPACE/customers.db") rows (never deleted)"
echo "src/feature.go: $(test -e "$AEGIS_WORKSPACE/src/feature.go" && echo still present || echo removed - the agent created it)"
echo "notes.md:       kept the human's version -> $(tail -1 "$AEGIS_WORKSPACE/notes.md")"

grep -q "replicas: 3" "$AEGIS_WORKSPACE/config.yaml"
test "$(wc -l < "$AEGIS_WORKSPACE/customers.db")" -eq 500
test ! -e "$AEGIS_WORKSPACE/src/feature.go"
grep -q "Arjun" "$AEGIS_WORKSPACE/notes.md"
test -f "$AEGIS_REPO/config" # the agent could not reach the vault
printf '\n\033[1;32mDone: rogue agent contained, its changes undone, human work kept.\033[0m\n'
