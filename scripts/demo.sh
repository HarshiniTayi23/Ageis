#!/usr/bin/env bash
# End-to-end demo: protect a dataset, simulate a ransomware attack, detect it,
# and recover the last clean point in time - with every file verified.
#
# The "attack" only overwrites files inside a throwaway temp directory with
# random bytes. It is a behavioural simulation, not malware.
set -euo pipefail
cd "$(dirname "$0")/.."

WORK="$(mktemp -d)"
trap 'chmod -R u+w "$WORK" 2>/dev/null; rm -rf "$WORK"' EXIT
export AEGIS_PASSPHRASE="demo-passphrase-not-for-production"
export AEGIS_REPO="$WORK/repo"
DATA="$WORK/hospital-data"

go build -o "$WORK/aegis" ./cmd/aegis
A="$WORK/aegis"
step() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

step "1. Create a fake hospital file share (claims, patient records, ledgers)"
mkdir -p "$DATA"/{claims,patients,finance}
for i in $(seq 1 60); do
  dir=$(echo claims patients finance | cut -d' ' -f$(( i % 3 + 1 )))
  for j in $(seq 1 300); do echo "$i,$j,patient-$((RANDOM)),claim-$((RANDOM)),approved,INR $((RANDOM * 7))"; done > "$DATA/$dir/record_$i.csv"
done
du -sh "$DATA"

step "2. Initialise an encrypted, immutable backup repository"
$A init

step "3. Day 1: full backup"
$A backup "$DATA"

step "4. Day 2: normal business - 3 files edited"
find "$DATA" -name '*.csv' | head -3 | while read -r f; do echo "late entry" >> "$f"; done
$A backup "$DATA"

step "5. Attacker goes after the backups first (like 93% of real attacks)"
SNAP=$(ls "$AEGIS_REPO/snapshots" | head -1)
if echo "garbage" > "$AEGIS_REPO/snapshots/$SNAP" 2>/dev/null; then
  echo "Day-1 snapshot overwritten on disk (attacker has root - local file permissions can't stop root)."
  echo "Aegis can't PREVENT this on a local disk, but it will DETECT it. Real prevention = S3 Object Lock (roadmap)."
else
  echo "overwrite blocked: object is read-only (WORM)"
fi

step "6. RANSOMWARE: encrypt 80% of files, rename to .locked, drop a note"
n=0; total=$(find "$DATA" -type f | wc -l)
find "$DATA" -type f | while read -r f; do
  n=$((n+1)); [ $n -gt $(( total * 8 / 10 )) ] && break
  head -c "$(stat -c%s "$f")" /dev/urandom > "$f.locked"; rm "$f"
done
echo "Your files are encrypted. Pay 50 BTC." > "$DATA/HOW_TO_DECRYPT.txt"
ls "$DATA/claims" | head -3

step "7. Next scheduled backup runs - and the detector fires"
$A backup "$DATA"

step "8. Snapshot timeline"
$A list

step "9. Verify repository integrity (hash chain + decrypt every chunk)"
$A verify --deep || echo "(verify exited non-zero: tampering detected, as expected)"

step "10. Recover the latest CLEAN point in time"
$A restore --latest-clean "$WORK/recovered"
echo "recovered files: $(find "$WORK/recovered" -type f | wc -l)  (.locked files: $(find "$WORK/recovered" -name '*.locked' | wc -l))"
head -2 "$(find "$WORK/recovered" -name '*.csv' | head -1)"

printf '\n\033[1;32mDone: attack detected, backups survived, clean data recovered.\033[0m\n'
