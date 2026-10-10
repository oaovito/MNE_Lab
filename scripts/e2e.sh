#!/usr/bin/env bash
# Browser tests: starts a seeded MNE Lab (internal/app TestBrowserHarness)
# and runs web/tests/e2e against it in every language, both themes and two
# window sizes, in Portable USB Mode and Temporary Machine Mode, plus the
# phone flow. Screenshots and results go to out/e2e/ for visual review.
#
#   scripts/e2e.sh
#
# Needs the built interface (cd web && npm ci && npm run build) and Chromium
# (cd web && npx playwright-core install chromium); MNELAB_CHROMIUM may point
# to another Chromium build.
set -euo pipefail
cd "$(dirname "$0")/.."

out=$PWD/out/e2e
rm -rf "$out"
mkdir -p "$out"
work=$(mktemp -d)
pids=()
cleanup() {
  for d in "$work"/*/; do touch "$d/stop" 2>/dev/null || true; done
  for p in "${pids[@]}"; do wait "$p" 2>/dev/null || true; done
  rm -rf "$work"
}
trap cleanup EXIT

start() { # name mode
  mkdir -p "$work/$1"
  MNELAB_E2E_DIR="$work/$1" MNELAB_E2E_MODE=$2 go test ./internal/app -run '^TestBrowserHarness$' -count=1 -timeout 40m >"$out/harness-$1.log" 2>&1 &
  pids+=($!)
}
ready() {
  for _ in $(seq 1 600); do
    [ -f "$work/$1/base.txt" ] && return 0
    kill -0 "${pids[-1]}" 2>/dev/null || { cat "$out/harness-$1.log" >&2; return 1; }
    sleep 0.5
  done
  echo "harness $1 did not start" >&2
  return 1
}

status=0
run() { (cd web && node "$@") || status=1; }

start portable portable
ready portable
for lang in en pt-BR es; do
  run tests/e2e/desktop.mjs "$work/portable" "$out" "$lang" light 1366x768
  run tests/e2e/desktop.mjs "$work/portable" "$out" "$lang" dark 1024x600
  run tests/e2e/phone.mjs "$work/portable" "$out" "$lang"
done
run tests/e2e/mapping.mjs "$work/portable" "$out"
run tests/e2e/profiles.mjs "$work/portable" "$out"
run tests/e2e/selection.mjs "$work/portable" "$out"
run tests/e2e/inspection.mjs "$work/portable" "$out"
run tests/e2e/import.mjs "$work/portable" "$out"
run tests/e2e/methods.mjs "$work/portable" "$out"
run tests/e2e/counts.mjs "$work/portable" "$out"
run tests/e2e/timezones.mjs "$work/portable" "$out"
run tests/e2e/statistics.mjs "$work/portable" "$out"
MNELAB_E2E_POSTHOC=dunnett run tests/e2e/statistics.mjs "$work/portable" "$out"
MNELAB_E2E_EFFECT_CI=1 run tests/e2e/statistics.mjs "$work/portable" "$out"
run tests/e2e/statistics-mixed.mjs "$work/portable" "$out"
run tests/e2e/dts.mjs "$work/portable" "$out"

start temporary temporary
ready temporary
run tests/e2e/desktop.mjs "$work/temporary" "$out/temporary" pt-BR dark 1366x768

exit $status
