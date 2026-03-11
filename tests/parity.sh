#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

OVERALL=0

echo "Building compiler binary for parity runs..."
(
  cd ..
  go build -o httpdsl .
)

echo ""
echo "Running parity suite (IR backend)"
echo "============================"
log="/tmp/httpdsl-parity-ir.log"
echo ""
echo "[ir] running tests/run.sh"
if bash ./run.sh >"$log" 2>&1; then
  summary=$(grep -E "ALL TESTS PASSED|FAILED" "$log" | tail -n 1 || true)
  echo "[ir] PASS ${summary}"
else
  rc=$?
  summary=$(grep -E "ALL TESTS PASSED|FAILED" "$log" | tail -n 1 || true)
  echo "[ir] FAIL (exit=$rc) ${summary}"
  OVERALL=1
fi
echo "[ir] log: $log"

echo ""
echo "Verifying golden checksums"
if ./golden/verify.sh; then
  echo "Golden verification: PASS"
else
  echo "Golden verification: FAIL"
  OVERALL=1
fi

exit "$OVERALL"
