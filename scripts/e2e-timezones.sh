#!/usr/bin/env bash
# Targeted timestamp validation against a fresh synthetic local profile.
set -euo pipefail
cd "$(dirname "$0")/.."
out=$PWD/out/e2e-timezones
mkdir -p "$out"
work=$(mktemp -d)
MNELAB_E2E_DIR="$work" go test ./internal/app -run '^TestBrowserHarness$' -count=1 -timeout 15m >"$out/harness.log" 2>&1 &
pid=$!
cleanup() {
  touch "$work/stop"
  wait "$pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT
for _ in $(seq 1 600); do
  [ -f "$work/base.txt" ] && break
  kill -0 "$pid" 2>/dev/null || { cat "$out/harness.log" >&2; exit 1; }
  sleep 0.5
done
[ -f "$work/base.txt" ] || { echo 'timestamp harness did not start' >&2; exit 1; }
(cd web && node tests/e2e/timezones.mjs "$work" "$out")
