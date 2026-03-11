#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
IN="checksums.txt"

if [[ ! -f "$IN" ]]; then
  echo "Missing $IN. Run ./generate.sh first." >&2
  exit 1
fi

(
  cd "$ROOT"
  go build -o httpdsl .
)

failed=0
while read -r fixture expected; do
  [[ -z "${fixture:-}" ]] && continue
  [[ "${fixture:0:1}" == "#" ]] && continue

  fixture_dir="$(dirname "$fixture")"
  fixture_file="$(basename "$fixture")"
  actual=$(
    cd "$ROOT/$fixture_dir" && \
      "$ROOT/httpdsl" emit "$fixture_file" | shasum -a 256 | awk '{print $1}'
  )
  if [[ "$actual" != "$expected" ]]; then
    echo "Mismatch: fixture=$fixture"
    echo "  expected: $expected"
    echo "  actual:   $actual"
    failed=1
  fi
done <"$IN"

exit "$failed"
