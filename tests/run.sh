#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Load .env if present
[[ -f .env ]] && set -a && source .env && set +a

PORT=${TEST_PORT:-9999}
BASE="http://localhost:$PORT"
PASSED=0
FAILED=0
SKIPPED=0
FAILURES=""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

# Build
echo "Building test server..."
cd ..
go run main.go build tests/test_server.httpdsl 2>&1
BIN="./test_server"
cd tests
BIN="../test_server"
echo ""

# Start server
$BIN &
SRV_PID=$!
trap "kill $SRV_PID 2>/dev/null; rm -f ../test_server" EXIT
sleep 1

# Check server is up
if ! curl -sf "$BASE/tests" > /dev/null 2>&1; then
    echo -e "${RED}Server failed to start${NC}"
    exit 1
fi

run_test() {
    local method="$1"
    local path="$2"
    local url="$BASE$path"
    local extra_args="${3:-}"
    local display="$method $path"

    local resp
    if [[ "$method" == "POST" ]]; then
        resp=$(eval curl -sf -X POST $extra_args "$url" 2>/dev/null) || { 
            echo -e "  ${RED}FAIL${NC} $display (curl error)"
            FAILED=$((FAILED + 1))
            FAILURES="$FAILURES\n  $display"
            return
        }
    else
        resp=$(curl -sf "$url" 2>/dev/null) || {
            echo -e "  ${RED}FAIL${NC} $display (curl error)"
            FAILED=$((FAILED + 1))
            FAILURES="$FAILURES\n  $display"
            return
        }
    fi

    local pass
    pass=$(echo "$resp" | jq -r '.pass' 2>/dev/null)
    
    if [[ "$pass" == "true" ]]; then
        local check_count
        check_count=$(echo "$resp" | jq '.checks | length' 2>/dev/null)
        echo -e "  ${GREEN}PASS${NC} $display ($check_count checks)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display"
        # Show failed checks
        echo "$resp" | jq -c '.checks[] | select(.pass == false)' 2>/dev/null | while read -r check; do
            local tname=$(echo "$check" | jq -r '.test')
            local expected=$(echo "$check" | jq -r '.expected')
            local actual=$(echo "$check" | jq -r '.actual')
            echo -e "         ${RED}✗${NC} $tname: expected=$expected actual=$actual"
        done
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}

run_redirect_test() {
    local path="$1"
    local url="$BASE$path"
    local display="GET $path (redirect)"
    
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null)
    
    if [[ "$status" == "301" || "$status" == "302" ]]; then
        echo -e "  ${GREEN}PASS${NC} $display (HTTP $status)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (expected 301/302, got $status)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}

run_raw_test() {
    local display="$1"
    local resp="$2"

    if [[ -z "$resp" ]]; then
        echo -e "  ${RED}FAIL${NC} $display (curl error)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
        return
    fi

    local pass
    pass=$(echo "$resp" | jq -r '.pass' 2>/dev/null)
    if [[ "$pass" == "true" ]]; then
        local check_count
        check_count=$(echo "$resp" | jq '.checks | length' 2>/dev/null)
        echo -e "  ${GREEN}PASS${NC} $display ($check_count checks)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display"
        echo "$resp" | jq -c '.checks[] | select(.pass == false)' 2>/dev/null | while read -r check; do
            local tname=$(echo "$check" | jq -r '.test')
            local expected=$(echo "$check" | jq -r '.expected')
            local actual=$(echo "$check" | jq -r '.actual')
            echo -e "         ${RED}✗${NC} $tname: expected=$expected actual=$actual"
        done
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}

run_cors_test() {
    local path="$1"
    local url="$BASE$path"
    local display="OPTIONS $path (CORS)"
    
    local headers
    headers=$(curl -sI -X OPTIONS "$url" 2>/dev/null)
    
    if echo "$headers" | grep -qi 'Access-Control-Allow-Origin'; then
        echo -e "  ${GREEN}PASS${NC} $display"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (no CORS headers)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}

echo "Running tests..."
echo ""

# Type tests
echo "Types:"
run_test GET /test/types/conversions
run_test GET /test/types/int-base
run_test GET /test/types/null
echo ""

# Math tests
echo "Math:"
run_test GET /test/math/abs
run_test GET /test/math/ceil-floor
run_test GET /test/math/round
run_test GET /test/math/rand
echo ""

# String tests
echo "Strings:"
run_test GET /test/strings/basic
run_test GET /test/strings/transform
run_test GET /test/strings/search
run_test GET /test/strings/template
run_test GET /test/strings/regex
echo ""

# Collection tests
echo "Collections:"
run_test GET /test/collections/sort
run_test GET /test/collections/sort-by
run_test GET /test/collections/functional
run_test GET /test/collections/array-ops
run_test GET /test/collections/object-ops
echo ""

# Encoding tests
echo "Encoding:"
run_test GET /test/encoding/base64
run_test GET /test/encoding/url
run_test GET /test/encoding/json
run_test GET /test/encoding/hash
echo ""

# Crypto tests
echo "Crypto:"
run_test GET /test/crypto/uuid
run_test GET /test/crypto/cuid2
run_test GET /test/crypto/hmac
run_test GET /test/crypto/jwt
echo ""

# DateTime tests
echo "DateTime:"
run_test GET /test/datetime/date
run_test GET /test/datetime/format
run_test GET /test/datetime/strtotime
echo ""

# Language tests
echo "Language:"
run_test GET /test/language/closures
run_test GET /test/language/destructure-obj
run_test GET /test/language/destructure-arr
run_test GET /test/language/shorthand
run_test GET /test/language/try-catch
run_test GET /test/language/if-else
run_test GET /test/language/each
run_test GET /test/language/while
run_test GET /test/language/multi-return
echo ""

# Routing tests
echo "Routing:"
run_test GET /test/routing/params/hello
run_test GET "/test/routing/query?a=1&b=2"
run_test GET /test/routing/wildcard/a/b/c
run_cors_test /test/routing/cors
run_redirect_test /test/routing/redirect
echo ""

# Store tests
echo "Store:"
run_test GET /test/store/crud
run_test GET /test/store/incr
echo ""

# File I/O tests
echo "File I/O:"
run_test GET /test/io/readwrite
run_test GET /test/io/exists-delete
run_test GET /test/io/json-io
run_test GET /test/io/mkdir-list
echo ""

# Body parsing tests
echo "Body parsing:"
run_raw_test "POST /test/body/json" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"name":"Alice","age":30}' "$BASE/test/body/json" 2>/dev/null)"

run_raw_test "POST /test/body/form" \
    "$(curl -sf -X POST -d 'username=alice&password=s3cret&remember=on' "$BASE/test/body/form" 2>/dev/null)"

# Create a temp file for multipart upload
TMP_FILE=$(mktemp /tmp/hello_XXXX.txt)
echo "hello world" > "$TMP_FILE"

run_raw_test "POST /test/body/multipart" \
    "$(curl -sf -X POST \
        -F 'username=bob' \
        -F 'description=a test upload' \
        -F "avatar=@${TMP_FILE};filename=hello.txt" \
        "$BASE/test/body/multipart" 2>/dev/null)"

# Create two temp files for multi-file upload
TMP_FILE1=$(mktemp /tmp/f1_XXXX.txt)
TMP_FILE2=$(mktemp /tmp/f2_XXXX.txt)
echo "content1" > "$TMP_FILE1"
echo "content2" > "$TMP_FILE2"

run_raw_test "POST /test/body/multipart-multi" \
    "$(curl -sf -X POST \
        -F 'title=batch upload' \
        -F "docs=@${TMP_FILE1};filename=file1.txt" \
        -F "docs=@${TMP_FILE2};filename=file2.txt" \
        "$BASE/test/body/multipart-multi" 2>/dev/null)"

rm -f "$TMP_FILE" "$TMP_FILE1" "$TMP_FILE2"

echo ""

# Async tests
echo "Async:"
run_test GET /test/async/basic
echo ""

# SQLite tests
echo "SQLite:"
run_test GET /test/sqlite/crud
run_test GET /test/sqlite/types
run_test GET /test/sqlite/empty
echo ""

# Summary
TOTAL=$((PASSED + FAILED + SKIPPED))
echo "========================================"
if [[ $FAILED -eq 0 ]]; then
    echo -e "${GREEN}ALL TESTS PASSED${NC} ($PASSED/$TOTAL)"
else
    echo -e "${RED}$FAILED FAILED${NC}, ${GREEN}$PASSED passed${NC} ($TOTAL total)"
    echo -e "\nFailed tests:$FAILURES"
fi
echo "========================================"

exit $FAILED
