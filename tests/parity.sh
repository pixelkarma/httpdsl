#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

BACKENDS=(legacy ir)
OVERALL=0

echo "Building compiler binary for parity runs..."
(
  cd ..
  go build -o httpdsl .
)

echo ""
echo "Running backend parity suite"
echo "============================"

for backend in "${BACKENDS[@]}"; do
  log="/tmp/httpdsl-parity-${backend}.log"
  echo ""
  echo "[$backend] running tests/run.sh"

  if HTTPDSL_BACKEND="$backend" bash ./run.sh >"$log" 2>&1; then
    summary=$(grep -E "ALL TESTS PASSED|FAILED" "$log" | tail -n 1 || true)
    echo "[$backend] PASS ${summary}"
  else
    rc=$?
    summary=$(grep -E "ALL TESTS PASSED|FAILED" "$log" | tail -n 1 || true)
    echo "[$backend] FAIL (exit=$rc) ${summary}"
    OVERALL=1
  fi
  echo "[$backend] log: $log"
done

echo ""
echo "Verifying golden checksums"
if ./golden/verify.sh; then
  echo "Golden verification: PASS"
else
  echo "Golden verification: FAIL"
  OVERALL=1
fi

exit "$OVERALL"
