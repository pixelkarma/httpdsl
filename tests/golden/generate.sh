#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT="checksums.txt"
FIXTURES=(
  "tests/golden/fixtures/basic.httpdsl"
  "tests/golden/fixtures/session.httpdsl"
  "tests/golden/fixtures/sse.httpdsl"
)
BACKENDS=(legacy ir)

(
  cd "$ROOT"
  go build -o httpdsl .
)

tmp="$(mktemp)"
{
  echo "# HTTPDSL generated-source checksums"
  echo "# Format: <backend> <fixture> <sha256>"

  for backend in "${BACKENDS[@]}"; do
    for fixture in "${FIXTURES[@]}"; do
      fixture_dir="$(dirname "$fixture")"
      fixture_file="$(basename "$fixture")"
      hash=$(
        cd "$ROOT/$fixture_dir" && \
          HTTPDSL_BACKEND="$backend" "$ROOT/httpdsl" emit "$fixture_file" | shasum -a 256 | awk '{print $1}'
      )
      printf "%s %s %s\n" "$backend" "$fixture" "$hash"
    done
  done
} >"$tmp"

mv "$tmp" "$OUT"
echo "Wrote $OUT"
