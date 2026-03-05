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

# Cleanup
rm -f /tmp/httpdsl_test_store.db

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
trap "kill -TERM $SRV_PID 2>/dev/null; rm -f ../test_server" EXIT
rm -f /tmp/httpdsl_shutdown_proof.txt
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
run_test GET /test/crypto/hash-password/bcrypt
run_test GET /test/crypto/hash-password/bcrypt-cost
run_test GET /test/crypto/hash-password/argon2
run_test GET /test/crypto/hash-password/argon2-opts
run_test GET /test/crypto/hash-password/cross-verify

echo ""
echo "Validation:"
run_test GET /test/validation/helpers

run_raw_test "POST /test/validation/schema/valid" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"name":"Alice","email":"alice@test.com","age":25}' "$BASE/test/validation/schema/valid" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/required" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"name":"Alice"}' "$BASE/test/validation/schema/required" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/types" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"str_field":"hello","int_field":42,"num_field":3.14,"bool_field":true,"arr_field":[1,2],"obj_field":{"k":"v"}}' "$BASE/test/validation/schema/types" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/type-fail" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"count":"not_int","flag":"not_bool"}' "$BASE/test/validation/schema/type-fail" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/min-max" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"name":"AB","score":200}' "$BASE/test/validation/schema/min-max" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/in-regex" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"role":"admin","code":"ABC123"}' "$BASE/test/validation/schema/in-regex" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/in-regex-fail" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"role":"hacker","code":"bad"}' "$BASE/test/validation/schema/in-regex-fail" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/optional" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{}' "$BASE/test/validation/schema/optional" 2>/dev/null)"

run_raw_test "POST /test/validation/schema/email-url-uuid" \
    "$(curl -sf -X POST -H 'Content-Type: application/json' -d '{"email":"a@b.com","website":"https://x.com","id":"550e8400-e29b-41d4-a716-446655440000"}' "$BASE/test/validation/schema/email-url-uuid" 2>/dev/null)"
echo ""

# Static file tests
echo "Static files:"

# Test: static file served
display="GET /static/style.css (static file)"
result=$(curl -sf "$BASE/static/style.css" 2>/dev/null) || true
if [ "$result" = "body{color:red}" ]; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (got: $result)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi

# Test: index.html served for directory
display="GET /static/ (index.html)"
result=$(curl -sf "$BASE/static/" 2>/dev/null) || true
if echo "$result" | grep -q '<h1>Hello</h1>'; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (got: $result)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi

# Test: nested static file
display="GET /static/sub/data.json (nested)"
result=$(curl -sf "$BASE/static/sub/data.json" 2>/dev/null) || true
if echo "$result" | jq -e '.data == "test"' > /dev/null 2>&1; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (got: $result)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi

# Test: directory listing disabled (no index.html in sub/)
display="GET /static/sub/ (no dir listing → 404)"
status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/static/sub/")
if [ "$status" = "404" ]; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (status: $status)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi

# Test: missing static file
display="GET /static/nope.txt (missing → 404)"
status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/static/nope.txt")
if [ "$status" = "404" ]; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (status: $status)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi
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

# Builtin tests
echo "Builtins:"
run_test GET /test/builtins/type-fn
run_test GET /test/builtins/reduce
run_test GET /test/builtins/has-keys-values
run_test GET /test/builtins/index-of
run_test GET /test/builtins/starts-ends
run_test GET /test/builtins/repeat-fn
run_test GET /test/builtins/delete-fn
run_test GET /test/builtins/json-parse-stringify
run_test GET /test/builtins/date-parse
run_test GET /test/builtins/env-fn
run_test GET /test/builtins/sleep-fn
run_test GET /test/builtins/now-fns
echo ""

# Operator tests
echo "Operators:"
run_test GET /test/operators/compound-assign
run_test GET /test/operators/modulo
run_test GET /test/operators/string-concat
run_test GET /test/operators/comparisons
run_test GET /test/operators/logical
echo ""

# Response type tests
echo "Response types:"
{
    display="GET /test/response/text"
    body=$(curl -sf -D /tmp/resp_text_hdrs.txt "$BASE/test/response/text" 2>/dev/null)
    ct=$(grep -i '^content-type:' /tmp/resp_text_hdrs.txt | tr -d '\r')
    rm -f /tmp/resp_text_hdrs.txt
    if echo "$ct" | grep -qi 'text/plain' && [[ "$body" == "plain text response" ]]; then
        echo -e "  ${GREEN}PASS${NC} $display (content-type + body)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (ct=$ct body=$body)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}
{
    display="GET /test/response/html"
    body=$(curl -sf -D /tmp/resp_html_hdrs.txt "$BASE/test/response/html" 2>/dev/null)
    ct=$(grep -i '^content-type:' /tmp/resp_html_hdrs.txt | tr -d '\r')
    rm -f /tmp/resp_html_hdrs.txt
    if echo "$ct" | grep -qi 'text/html' && [[ "$body" == "<h1>Hello</h1>" ]]; then
        echo -e "  ${GREEN}PASS${NC} $display (content-type + body)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (ct=$ct body=$body)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}
{
    display="GET /test/response/status"
    status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/test/response/status" 2>/dev/null)
    if [[ "$status" == "201" ]]; then
        echo -e "  ${GREEN}PASS${NC} $display (HTTP 201)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (expected 201, got $status)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}
{
    display="GET /test/response/headers"
    curl -sf -D /tmp/resp_hdrs.txt "$BASE/test/response/headers" > /dev/null 2>&1
    x_custom=$(grep -i 'X-Custom' /tmp/resp_hdrs.txt | tr -d '\r')
    x_reqid=$(grep -i 'X-Request-Id' /tmp/resp_hdrs.txt | tr -d '\r')
    rm -f /tmp/resp_hdrs.txt
    if echo "$x_custom" | grep -q 'test-value' && echo "$x_reqid" | grep -q 'abc-123'; then
        echo -e "  ${GREEN}PASS${NC} $display (2 custom headers)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (X-Custom=$x_custom X-Request-Id=$x_reqid)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}
{
    display="GET /test/response/cookies"
    curl -sf -D /tmp/resp_cookies.txt "$BASE/test/response/cookies" > /dev/null 2>&1
    simple=$(grep -i 'Set-Cookie.*simple=hello' /tmp/resp_cookies.txt | head -1)
    complex=$(grep -i 'Set-Cookie.*complex=world' /tmp/resp_cookies.txt | head -1)
    rm -f /tmp/resp_cookies.txt
    if [[ -n "$simple" && -n "$complex" ]]; then
        checks=1
        [[ -n "$complex" ]] && checks=$((checks + 1))
        echo "$complex" | grep -qi 'HttpOnly' && checks=$((checks + 1))
        echo -e "  ${GREEN}PASS${NC} $display ($checks checks)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (simple=$simple complex=$complex)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}
echo ""

# Request inspection tests
echo "Request inspection:"
run_raw_test "GET /test/request/headers" \
    "$(curl -sf -H 'X-Test-Header: test-value' "$BASE/test/request/headers" 2>/dev/null)"
run_raw_test "GET /test/request/ip" \
    "$(curl -sf "$BASE/test/request/ip" 2>/dev/null)"
run_raw_test "GET /test/request/cookies" \
    "$(curl -sf -b 'session=abc123' "$BASE/test/request/cookies" 2>/dev/null)"
echo ""

# HTTP method tests
echo "HTTP methods:"
run_raw_test "PUT /test/methods/put" \
    "$(curl -sf -X PUT "$BASE/test/methods/put" 2>/dev/null)"
run_raw_test "DELETE /test/methods/delete" \
    "$(curl -sf -X DELETE "$BASE/test/methods/delete" 2>/dev/null)"
run_raw_test "PATCH /test/methods/patch" \
    "$(curl -sf -X PATCH -H 'Content-Type: application/json' -d '{"field":"updated"}' "$BASE/test/methods/patch" 2>/dev/null)"
echo ""

# Fetch tests
echo "Fetch:"
run_test GET /test/fetch/basic
run_test GET /test/fetch/post
echo ""

# Error handling tests
echo "Error handling:"
run_test GET /test/errors/throw-catch
run_test GET /test/errors/throw-object
run_test GET /test/errors/nested-try
echo ""

# File I/O extras
echo "File I/O extras:"
run_test GET /test/io-extra/append
run_test GET /test/io-extra/chmod
echo ""

# Async extras
echo "Async extras:"
run_test GET /test/async-extra/race
run_test GET /test/async-extra/multi-await
echo ""

# Closure / HOF tests
echo "Closures/HOF:"
run_test GET /test/closures/mutable-capture
run_test GET /test/closures/fn-as-arg
run_test GET /test/closures/fn-return-fn
run_test GET /test/closures/each-closure
echo ""

# Log / print tests
echo "Log/Print:"
run_test GET /test/log/all-levels
echo ""

# Fetch extras
echo "Fetch extras:"
run_test GET /test/fetch/error
run_test GET /test/fetch/headers
echo ""

# PostgreSQL tests
echo "PostgreSQL:"
run_test GET /test/postgres/crud
run_test GET /test/postgres/types
echo ""

# MySQL tests
echo "MySQL:"
run_test GET /test/mysql/crud
run_test GET /test/mysql/types
echo ""

# Builtin functions
echo "Builtins:"
run_test GET /test/builtins/find-some-every
run_test GET /test/builtins/pluck-group
run_test GET /test/builtins/sum-min-max
run_test GET /test/builtins/chunk-range
run_test GET /test/builtins/string-helpers
echo ""

# Scheduled tasks
echo "Scheduled tasks:"
sleep 2  # wait for at least 1 tick
run_test GET /test/scheduled/ticks
echo ""

# Expression tests
echo "Expressions:"
run_test GET /test/expressions/ternary
run_test GET /test/expressions/nullish
run_test GET /test/expressions/logical-or
run_test GET /test/expressions/logical-and
echo ""

# Switch tests
echo "Switch:"
run_test GET /test/switch/basic/a
run_test GET /test/switch/multi
run_test GET /test/switch/default
run_test GET /test/switch/int
run_test GET /test/switch/no-default
echo ""

# Timeout tests
echo "Timeout:"

# Fast route with timeout should return 200
display="GET /test/timeout/fast (within timeout → 200)"
status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/test/timeout/fast")
if [ "$status" = "200" ]; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (status: $status)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi

# Slow route with timeout should return 504
display="GET /test/timeout/slow (exceeds timeout → 504)"
status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/test/timeout/slow")
if [ "$status" = "504" ]; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (status: $status)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi
echo ""

# Middleware tests
echo "Middleware:"
run_test GET /test/middleware/global-before

# Auth before — with valid token
run_raw_test "GET /test/middleware/auth/profile (authed)" \
    "$(curl -sf -H 'Authorization: Bearer secret123' "$BASE/test/middleware/auth/profile" 2>/dev/null)"

# Auth before — without token (should 401)
{
    display="GET /test/middleware/auth/profile (no auth → 401)"
    status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/test/middleware/auth/profile" 2>/dev/null)
    if [[ "$status" == "401" ]]; then
        echo -e "  ${GREEN}PASS${NC} $display"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (expected 401, got $status)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}

# After block test — trigger then verify
curl -sf "$BASE/test/middleware/after/trigger" > /dev/null 2>&1
sleep 0.3  # let goroutine finish
run_test GET /test/middleware/after/verify
echo ""

# CORS detail test
echo "CORS details:"
{
    display="OPTIONS /test/cors/details"
    curl -sf -X OPTIONS -D /tmp/cors_hdrs.txt -o /dev/null "$BASE/test/cors/details" 2>/dev/null
    status=$(grep 'HTTP/' /tmp/cors_hdrs.txt | awk '{print $2}' | tr -d '\r')
    origin=$(grep -i 'Access-Control-Allow-Origin' /tmp/cors_hdrs.txt | tr -d '\r')
    methods=$(grep -i 'Access-Control-Allow-Methods' /tmp/cors_hdrs.txt | tr -d '\r')
    allow_hdrs=$(grep -i 'Access-Control-Allow-Headers' /tmp/cors_hdrs.txt | tr -d '\r')
    rm -f /tmp/cors_hdrs.txt
    checks=0
    ok=true
    if echo "$origin" | grep -q '\*'; then checks=$((checks + 1)); else ok=false; fi
    if echo "$methods" | grep -qi 'GET'; then checks=$((checks + 1)); else ok=false; fi
    if echo "$methods" | grep -qi 'POST'; then checks=$((checks + 1)); else ok=false; fi
    if echo "$allow_hdrs" | grep -qi 'Content-Type'; then checks=$((checks + 1)); else ok=false; fi
    if [[ "$status" == "204" ]]; then checks=$((checks + 1)); else ok=false; fi
    if $ok; then
        echo -e "  ${GREEN}PASS${NC} $display ($checks checks)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (origin=$origin methods=$methods headers=$allow_hdrs status=$status)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
}
echo ""

# Gzip tests
echo "Gzip:"
display="GET /test/types/conversions (gzip Content-Encoding)"
encoding=$(curl -s -H "Accept-Encoding: gzip" -D - -o /dev/null "$BASE/test/types/conversions" 2>/dev/null | grep -i 'Content-Encoding' | tr -d '\r')
if echo "$encoding" | grep -qi 'gzip'; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display (header: $encoding)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi

display="GET /test/types/conversions (gzip decompresses ok)"
result=$(curl -sf --compressed "$BASE/test/types/conversions" 2>/dev/null)
pass=$(echo "$result" | jq -r '.pass' 2>/dev/null)
if [ "$pass" = "true" ]; then
    echo -e "  ${GREEN}PASS${NC} $display"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} $display"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  $display"
fi
echo ""

# Session tests
echo "Sessions:"
# Login — creates session, returns cookie
resp=$(curl -sf -c /tmp/httpdsl_test_cookies.txt -X POST \
  -H "Content-Type: application/json" \
  -d '{"user_id": 42, "role": "admin"}' \
  "$BASE/test/session/login" 2>/dev/null) || true
pass=$(echo "$resp" | jq -r '.pass' 2>/dev/null)
if [[ "$pass" == "true" ]]; then
    echo -e "  ${GREEN}PASS${NC} POST /test/session/login (1 checks)"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} POST /test/session/login"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  POST /test/session/login"
fi
# Check — reads session from cookie
resp=$(curl -sf -b /tmp/httpdsl_test_cookies.txt "$BASE/test/session/check" 2>/dev/null) || true
pass=$(echo "$resp" | jq -r '.pass' 2>/dev/null)
if [[ "$pass" == "true" ]]; then
    check_count=$(echo "$resp" | jq '.checks | length' 2>/dev/null)
    echo -e "  ${GREEN}PASS${NC} GET /test/session/check ($check_count checks)"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} GET /test/session/check"
    echo "$resp" | jq -c '.checks[] | select(.pass == false)' 2>/dev/null | while read -r check; do
        tname=$(echo "$check" | jq -r '.test')
        expected=$(echo "$check" | jq -r '.expected')
        actual=$(echo "$check" | jq -r '.actual')
        echo -e "         ${RED}x${NC} $tname: expected=$expected actual=$actual"
    done
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  GET /test/session/check"
fi
# Destroy
resp=$(curl -sf -b /tmp/httpdsl_test_cookies.txt -X POST "$BASE/test/session/destroy" 2>/dev/null) || true
pass=$(echo "$resp" | jq -r '.pass' 2>/dev/null)
if [[ "$pass" == "true" ]]; then
    echo -e "  ${GREEN}PASS${NC} POST /test/session/destroy (1 checks)"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} POST /test/session/destroy"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  POST /test/session/destroy"
fi
# After destroy — session should be gone
resp=$(curl -sf -b /tmp/httpdsl_test_cookies.txt "$BASE/test/session/after-destroy" 2>/dev/null) || true
pass=$(echo "$resp" | jq -r '.pass' 2>/dev/null)
if [[ "$pass" == "true" ]]; then
    check_count=$(echo "$resp" | jq '.checks | length' 2>/dev/null)
    echo -e "  ${GREEN}PASS${NC} GET /test/session/after-destroy ($check_count checks)"
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}FAIL${NC} GET /test/session/after-destroy"
    echo "$resp" | jq -c '.checks[] | select(.pass == false)' 2>/dev/null | while read -r check; do
        tname=$(echo "$check" | jq -r '.test')
        expected=$(echo "$check" | jq -r '.expected')
        actual=$(echo "$check" | jq -r '.actual')
        echo -e "         ${RED}x${NC} $tname: expected=$expected actual=$actual"
    done
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  GET /test/session/after-destroy"
fi
rm -f /tmp/httpdsl_test_cookies.txt
echo ""

# Store sync tests
echo "Store sync:"
run_test GET /test/store-sync/write
sleep 3  # wait for flush
run_test GET /test/store-sync/verify-db
run_test GET /test/store-sync/delete
sleep 3  # wait for flush
run_test GET /test/store-sync/verify-delete
echo ""

# Server stats test
echo "Server stats:"
run_test GET /test/server-stats
echo ""

# Init block tests
echo "Init blocks:"
run_test GET /test/init/basic
run_test GET /test/init/functions
run_test GET /test/init/mutate
run_test GET /test/init/after-mutate
echo ""

# Error handler tests
echo "Error handlers:"
{
    display="GET /nonexistent (custom 404)"
    # Hit a route that doesn't exist — should get our custom error handler
    status=$(curl -s -o /tmp/err404_resp.json -w "%{http_code}" "$BASE/this/does/not/exist" 2>/dev/null)
    resp=$(cat /tmp/err404_resp.json 2>/dev/null)
    err_msg=$(echo "$resp" | jq -r '.error' 2>/dev/null)
    err_path=$(echo "$resp" | jq -r '.path' 2>/dev/null)
    err_method=$(echo "$resp" | jq -r '.method' 2>/dev/null)

    if [[ "$status" == "404" && "$err_msg" == "not found" && "$err_path" == "/this/does/not/exist" && "$err_method" == "GET" ]]; then
        echo -e "  ${GREEN}PASS${NC} $display (3 checks)"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} $display (status=$status error=$err_msg path=$err_path method=$err_method)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  $display"
    fi
    rm -f /tmp/err404_resp.json
}
echo ""

# Shutdown test — kill server gracefully and check proof file
echo "Shutdown:"
kill -TERM $SRV_PID 2>/dev/null
sleep 2
if [[ -f /tmp/httpdsl_shutdown_proof.txt ]]; then
    content=$(cat /tmp/httpdsl_shutdown_proof.txt)
    if [[ "$content" == "shutdown_ok" ]]; then
        echo -e "  ${GREEN}PASS${NC} shutdown block executed"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}FAIL${NC} shutdown block (wrong content: $content)"
        FAILED=$((FAILED + 1))
        FAILURES="$FAILURES\n  shutdown block"
    fi
    rm -f /tmp/httpdsl_shutdown_proof.txt
else
    echo -e "  ${RED}FAIL${NC} shutdown block (proof file not found)"
    FAILED=$((FAILED + 1))
    FAILURES="$FAILURES\n  shutdown block"
fi
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
