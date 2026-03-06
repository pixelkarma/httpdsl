# HTTPDSL Language Documentation

HTTPDSL is a domain-specific language that compiles to standalone, native Go HTTP server binaries. You write `.httpdsl` files. The compiler produces a single Go source file. `go build` turns it into a static binary with zero runtime dependencies.

This is the complete language reference.

---

## Table of Contents

### I. Getting Started

- [**The Compiler**](compiler.md) — Installing, building, and using the `httpdsl` CLI
  - Building from source
  - `httpdsl build` command
  - Compilation model (`.httpdsl` → Go source → native binary)
  - Go version requirements
  - Project structure conventions

- [**Hello World**](hello-world.md) — Your first HTTPDSL server
  - Minimal server example
  - Running the compiled binary
  - The request/response cycle

### II. Language Fundamentals

- [**Server Configuration**](server.md) — The `server { }` block
  - `port` — listen port
  - `gzip` — response compression
  - `throttle_requests_per_second` — per-IP rate limiting
  - `static` — file serving mounts
  - `templates` — Go html/template directory
  - `cors` — cross-origin resource sharing
  - `session` — server-side sessions and CSRF

- [**Types & Values**](types.md) — The value system
  - Strings, integers, floats, booleans, null
  - Arrays (ordered lists)
  - Hashes (key-value maps)
  - Type coercion rules
  - Template strings (backtick interpolation)

- [**Variables & Assignment**](variables.md) — Declaring and mutating state
  - Assignment (`=`, `+=`, `-=`)
  - Destructuring — array (`[a, b] = expr`) and object (`{a, b} = expr`)
  - Scoping rules
  - Reserved words and naming constraints

- [**Operators**](operators.md) — Arithmetic, comparison, and logic
  - Arithmetic (`+`, `-`, `*`, `/`, `%`)
  - Comparison (`==`, `!=`, `<`, `>`, `<=`, `>=`)
  - Logical (`&&`, `||`, `!`)
  - Ternary (`condition ? a : b`)
  - Nullish coalescing (`??`)

- [**Control Flow**](control-flow.md) — Branching and looping
  - `if` / `else`
  - `switch` / `case` / `default`
  - `while` loops
  - `each ... in` iteration
  - `break`, `continue`

- [**Functions**](functions.md) — Defining and calling functions
  - `fn` declarations
  - Parameters and return values
  - Functions as values
  - Closures

- [**Error Handling**](errors.md) — Exceptions and recovery
  - `try` / `catch`
  - `throw`
  - Error propagation

### III. HTTP Server

- [**Routes**](routes.md) — Defining HTTP endpoints
  - `route METHOD "/path" { }` syntax
  - HTTP methods (`GET`, `POST`, `PUT`, `DELETE`, `PATCH`)
  - Path parameters (`:param`, `*wildcard`)
  - Content type enforcement (`json`, `text`, `form`)
  - Route-level `timeout`
  - Route-level `csrf false` opt-out

- [**Request Object**](request.md) — Accessing incoming data
  - `request.method` — HTTP method
  - `request.path` — URL path
  - `request.params` — path parameters
  - `request.query` — query string parameters
  - `request.data` — parsed body (JSON, form, text)
  - `request.headers` — request headers
  - `request.cookies` — request cookies
  - `request.ip` — client IP address
  - `request.bearer` — Bearer token from Authorization header
  - `request.basic` — `{username, password}` from Basic auth

- [**Response Object**](response.md) — Shaping the output
  - `response.body` — response payload
  - `response.status` — HTTP status code
  - `response.type` — content type (`"json"`, `"text"`, `"html"`)
  - `response.headers` — response headers hash
  - `response.cookies` — setting cookies
  - Automatic JSON serialization

- [**Route Groups**](groups.md) — Prefixed route collections
  - `group "/prefix" { }` syntax
  - Nested route definitions
  - `before { }` — group-level pre-middleware
  - `after { }` — group-level post-middleware

- [**Middleware**](middleware.md) — Before and after hooks
  - Top-level `before { }` / `after { }`
  - Group-scoped `before { }` / `after { }`
  - Execution order

- [**Error Pages**](error-pages.md) — Custom error responses
  - `error <status_code> { }` blocks
  - Custom 404, 500, etc.

- [**Templates**](templates.md) — Server-side HTML rendering
  - `server { templates "./dir" }` configuration
  - `render("template.gohtml", { data })` — as statement or expression
  - Go `html/template` syntax
  - Template data: `{{.Page.key}}`, `{{.Request.method}}`
  - CSRF helpers: `{{csrf_field}}`, `{{csrf_token}}`
  - Layouts, partials, and template inheritance
  - Compile-time embedding (no runtime filesystem access)

### IV. Security

- [**Sessions**](sessions.md) — Server-side session management
  - Configuration (`cookie`, `expires`, `secret`)
  - `session.key` — reading session values
  - `session.key = value` — writing session values
  - Cookie-based session IDs with HMAC signing

- [**CSRF Protection**](csrf.md) — Cross-site request forgery prevention
  - Enabling CSRF (`csrf true` in session config)
  - `csrf_safe_origins` — trusted origins
  - Automatic validation on `POST`, `PUT`, `DELETE`, `PATCH`
  - Token sources: `_csrf` form field, `X-CSRF-Token` header, `X-XSRF-Token` header, `_csrf` query param, `_csrf` in JSON body
  - `csrf_token()` — get current token
  - `csrf_field()` — get hidden input HTML
  - Per-route `csrf false` opt-out
  - Constant-time comparison (HMAC-based)

- [**Authentication Helpers**](auth.md) — Passwords, JWT, auth headers
  - `hash_password(plain)` / `verify_password(plain, hash)` — bcrypt
  - `jwt.sign(payload, secret)` / `jwt.verify(token, secret)` — HS256/HS384/HS512
  - `request.bearer` — extracting Bearer tokens
  - `request.basic` — extracting Basic auth credentials

- [**CORS**](cors.md) — Cross-origin resource sharing
  - `origins`, `methods`, `headers` configuration
  - Preflight handling

### V. Data & Storage

- [**Databases**](databases.md) — SQL and MongoDB
  - `db.open(driver, connection_string)` — connect to a database
  - Supported drivers: `sqlite`, `postgres`, `mysql`, `mongo`
  - `db.query(conn, sql, ...params)` — query rows
  - `db.exec(conn, sql, ...params)` — execute statements
  - `db.close(conn)` — close connection
  - MongoDB operations (`find`, `insert`, `update`, `delete`, `count`)
  - Parameterized queries

- [**Key-Value Store**](store.md) — In-memory key-value storage
  - `store.set(key, value)` — store a value
  - `store.set(key, value, ttl)` — store with TTL (seconds)
  - `store.get(key)` — retrieve (returns null if expired/missing)
  - `store.has(key)` — check existence
  - `store.delete(key)` — remove
  - `store.incr(key, amount)` / `store.incr(key, amount, ttl)` — atomic increment
  - `store.keys()` — list all keys
  - `store.clear()` — remove all
  - `store.sync("file.json")` — persist to JSON file
  - `store.sync(db, table)` — persist to SQL database
  - TTL expiration and background sweeper

- [**File Operations**](files.md) — Filesystem access
  - `file.read(path)` / `file.write(path, content)`
  - `file.append(path, content)`
  - `file.read_json(path)` / `file.write_json(path, data)`
  - `file.exists(path)` / `file.delete(path)`
  - `file.list(path)` / `file.mkdir(path)`
  - `file.chmod(path, mode)`

- [**Configuration**](env.md) — Runtime configuration
  - `.env` file loading (dotenv format, auto-loaded from working directory)
  - `env("NAME")` / `env("NAME", "default")` — read from `.env` file
  - `args` map — read `--key value` CLI flags
  - `help` block — define `-h` output
  - Built-in flags: `-h`, `-v`, `-e <path>`
  - Load order and precedence

### VI. Async & Concurrency

- [**Async / Await**](async.md) — Concurrent execution
  - `async expression` — spawn a concurrent task (returns a future)
  - `await(future)` — block until result is ready
  - `race(future1, future2, ...)` — first to complete wins
  - Async-compatible builtins (`fetch`, `exec`, etc.)

- [**Server-Sent Events**](sse.md) — Real-time streaming
  - `route SSE "/path" { }` — SSE endpoint declaration
  - `stream.send(event, data)` — send event to connected client
  - `stream.send(data)` — send with default "message" event
  - `stream.join(channel)` — subscribe client to a channel
  - `broadcast(event, data)` — send to all connected clients
  - `broadcast(event, data, channel)` — send to specific channel
  - Auto-cleanup on disconnect
  - Channel-based pub/sub architecture

### VII. Scheduled Tasks

- [**Timers & Cron**](scheduling.md) — Background task scheduling
  - `every N s/m/h { }` — interval-based (seconds, minutes, hours)
  - `every "cron_expression" { }` — cron-based (5-field: min hour dom month dow)
  - Cron syntax: `*`, ranges (`1-5`), steps (`*/5`), lists (`1,3,5`)
  - Runs alongside the HTTP server

### VIII. Lifecycle

- [**Shutdown**](shutdown.md) — Graceful shutdown hooks
  - `shutdown { }` block
  - Automatic store flushing
  - Session cleanup
  - Database connection closing
  - Signal handling (`SIGINT`, `SIGTERM`)

### IX. Builtin Functions Reference

- [**String Functions**](builtins/strings.md)
  - `len`, `trim`, `upper`, `lower`, `split`, `join`, `replace`
  - `starts_with`, `ends_with`, `contains`, `index_of`
  - `repeat`, `slice`, `pad_left`, `pad_right`
  - `truncate`, `capitalize`

- [**Array Functions**](builtins/arrays.md)
  - `len`, `append`, `slice`, `reverse`, `unique`, `flat`, `chunk`
  - `sort`, `sort_by`
  - `contains` / `has` / `includes`
  - `index_of`

- [**Hash Functions**](builtins/hashes.md)
  - `keys`, `values`, `merge`, `delete`, `has`

- [**Functional Iteration**](builtins/functional.md)
  - `map`, `filter`, `reduce`, `find`, `some`, `every`
  - `count`, `pluck`, `group_by`, `sum`, `min`, `max`

- [**Type Functions**](builtins/types.md)
  - `type`, `str`, `int`, `float`, `bool`

- [**Math Functions**](builtins/math.md)
  - `abs`, `ceil`, `floor`, `round`, `clamp`
  - `rand`, `range`

- [**Encoding Functions**](builtins/encoding.md)
  - `base64_encode`, `base64_decode`
  - `url_encode`, `url_decode`
  - `json.parse`, `json.stringify`

- [**Crypto Functions**](builtins/crypto.md)
  - `hash(algo, data)` — SHA-256, SHA-512, MD5
  - `hmac(algo, data, key)` — HMAC signing
  - `uuid()`, `cuid2()` — ID generation

- [**Validation Functions**](builtins/validation.md)
  - `validate(data, schema)` — schema-based validation
  - `is_email`, `is_url`, `is_uuid`, `is_numeric`

- [**Date & Time Functions**](builtins/datetime.md)
  - `date()`, `date(format)`
  - `date_format(timestamp, format)`
  - `date_parse(string, format)`
  - `strtotime(expression)` — relative time parsing

- [**HTTP Client**](builtins/fetch.md)
  - `fetch(url)` — GET request
  - `fetch(url, options)` — full request with method, headers, body
  - Response: `{status, body, headers, cookies}`

- [**Shell Execution**](builtins/exec.md)
  - `exec(command)` — run shell command (default 30s timeout)
  - `exec(command, timeout_seconds)` — run with custom timeout
  - Returns `{stdout, stderr, status, ok}`
  - Async compatible (`async exec(...)` → `await(...)`)

- [**Logging & Debugging**](builtins/logging.md)
  - `log(args...)` — print to stderr
  - `log_info`, `log_warn`, `log_error` — leveled logging
  - `print(args...)` — print to stdout
  - `sleep(ms)` — pause execution
  - `server_stats()` — runtime server metrics

- [**Navigation**](builtins/navigation.md)
  - `redirect(url)` — 302 redirect
  - `redirect(url, status)` — redirect with custom status code

---

> **Note:** Each linked page is a standalone document. Pages marked with `(builtins/...)` are function references with signatures, examples, and edge cases. Pages at the top level cover concepts, patterns, and configuration.
