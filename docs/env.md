# Configuration

- [.env File](#env-file)
  - [Format Rules](#format-rules)
- [env()](#env)
  - [Default Values](#default-values)
- [args Map](#args-map)
- [help Block](#help-block)
- [Built-in Flags](#built-in-flags)
- [Load Order](#load-order)
- [Server Block Limitations](#server-block-limitations)
- [Examples](#examples)
  - [API with CLI Configuration](#api-with-cli-configuration)
  - [Database Connection](#database-connection)
  - [Environment Detection](#environment-detection)
  - [.env File](#env-file)

Runtime configuration through `.env` files, CLI arguments, and built-in flags.

## .env File

HTTPDSL automatically loads a `.env` file from the working directory when the compiled binary starts. The file uses standard dotenv format:

```env
# Database
DATABASE_URL=sqlite:./app.db
DB_POOL_SIZE=5

# Server
SESSION_SECRET=change-me-in-production
API_KEY=sk_live_abc123

# Feature flags
FEATURE_BETA=true

# Quoted values
APP_NAME="My Application"
GREETING='Hello, world!'

# Export prefix (ignored, for shell compatibility)
export LOG_LEVEL=debug
```

### Format Rules

- **Comments**: Lines starting with `#` are ignored
- **Empty lines**: Ignored
- **`export` prefix**: Optional, stripped automatically
- **Double quotes**: Supports escape sequences (`\n`, `\t`, `\"`, `\\`)
- **Single quotes**: Literal strings, no escape processing
- **Unquoted values**: Taken as-is after trimming whitespace
- **No variable interpolation**: `$VAR` or `${VAR}` are treated as literal text

## env()

Read values from the `.env` file map:

```httpdsl
db_url = env("DATABASE_URL")
log_info(db_url)
```

Returns `null` if the key is not found.

### Default Values

Provide a fallback for missing keys:

```httpdsl
db_url = env("DATABASE_URL", "sqlite:./app.db")
log_level = env("LOG_LEVEL", "info")
max_upload = env("MAX_UPLOAD", "10")
```

> **Note:** `env()` reads from the `.env` file map only. It does **not** access OS environment variables.

## args Map

The `args` built-in is a read-only map populated from `--key value` CLI flags:

```bash
./myapp --mode production --db-path ./data.db
```

```httpdsl
mode = args["mode"] ?? "development"
db_path = args["db-path"] ?? "./app.db"
```

Every `--key` consumes the next argument as its value. A `--key` at the end of the command line (with no following argument) is set to `true`:

```bash
./myapp --mode production --verbose
```

```httpdsl
if args["verbose"] {
  log_info("verbose is on")
}
```

> **Note:** Port is handled by the built-in `-p` flag (see [Built-in Flags](#built-in-flags)), not through `args`. Use `args` for app-specific configuration.

> **Note:** `--key` always consumes the next argument as its value. `--foo --bar` sets `args["foo"]` to the string `"--bar"`, not `true`. Put value-less flags last.

## help Block

Define help text shown when the binary is run with `-h`:

```httpdsl
help `My API Server

A REST API for managing widgets.

Usage:
  ./myapp -p 8080 --db postgres://localhost/mydb

Options:
  --db <url>     Database connection URL
  --verbose      Enable verbose logging`
```

The `help` keyword takes a backtick string. This text is printed before the built-in flags when `-h` is used.

## Built-in Flags

Every compiled binary supports these flags:

| Flag | Description |
|------|-------------|
| `-p <port>` | Override listen port (also via `PORT` env var) |
| `-s <dir>` | Override primary static file directory |
| `-e <path>` | Load a specific `.env` file instead of `.env` |
| `-e none` | Skip `.env` loading entirely |
| `-a <domain>` | Enable Let's Encrypt autocert for domain (comma-separated for multiple) |
| `-ad <dir>` | Autocert cache directory (default: `.autocert`) |
| `-v` | Print `Built with httpdsl` and exit |
| `-h` | Print help text (if defined) and list built-in flags, then exit |

Port precedence: `-p` flag → `PORT` env var → compiled default from `server { port ... }`.

The following **environment variables** are also recognized:

| Variable | Description |
|----------|-------------|
| `PORT` | Override listen port |
| `SSL_CERT` | Path to TLS certificate file |
| `SSL_KEY` | Path to TLS private key file |
| `AUTOCERT_DOMAIN` | Enable Let's Encrypt for domain |
| `AUTOCERT_DIR` | Autocert cache directory |
| `HTTPS_REDIRECT` | Redirect HTTP to HTTPS (`true`/`false`, default: `true` when TLS active) |
| `WWW_REDIRECT` | Redirect non-www to www (`true`/`false`, default: `false`) |

SSL precedence: `SSL_CERT`/`SSL_KEY` env → compiled `ssl_cert`/`ssl_key` → no TLS.

Example output of `-h`:

```
My API Server

A REST API for managing widgets.

Flags:
  -p <port>   Listen port (default: 8080, or PORT env var)
  -s <dir>    Static file directory (default: ./public)
  -e <path>   Load env file (default: .env, "none" to skip)
  -a <domain> Let's Encrypt autocert for domain
  -ad <dir>   Autocert cache directory (default: .autocert)
  -v          Show version
  -h          Show this help

Environment variables:
  PORT             Override listen port
  SSL_CERT         Path to TLS certificate file
  SSL_KEY          Path to TLS private key file
  AUTOCERT_DOMAIN  Enable Let's Encrypt for domain
  AUTOCERT_DIR     Autocert cache directory
  HTTPS_REDIRECT   Redirect HTTP to HTTPS (true/false, default: true when TLS active)
  WWW_REDIRECT     Redirect non-www to www (true/false, default: false)
```

## Load Order

Configuration sources from weakest to strongest:

1. **Compiled defaults** — `server { port 8080 }` etc.
2. **`.env` file** — loaded at startup, populates the `env()` map
3. **OS environment variables** — `PORT`, `SSL_CERT`, `SSL_KEY`
4. **CLI flags** — `-p`, `-s`, and `--key value` args

The `env()` function and `args` map are separate namespaces. `env()` reads from `.env`, `args` reads from `--key value` CLI flags. Your code decides how to combine them:

```httpdsl
# CLI args override .env values
db_url = args["db"] ?? env("DATABASE_URL", "sqlite:./app.db")
mode = args["mode"] ?? env("MODE", "development")
```

Port, SSL, and static dir are handled automatically by built-in flags — no need to wire them up manually.

## Server Block Limitations

Most settings in the `server {}` block are parsed at **compile time** as literal values. You cannot use runtime expressions like `env()` in most of them.

The exceptions are `session.secret`, `ssl_cert`, `ssl_key`, `autocert`, and `autocert_dir`, which support runtime expressions:

```httpdsl
server {
  port 8080
  templates "./templates"
  static "/assets" "./public"
  ssl_cert env("SSL_CERT", "")
  ssl_key env("SSL_KEY", "")

  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")  # evaluated at runtime
  }
}
```

For port and static directory, use the built-in `-p` and `-s` flags instead. See [Built-in Flags](#built-in-flags) and [Server → Runtime Overrides](server.md#runtime-overrides).

## Examples

### API with CLI Configuration

```httpdsl
help `Widget API

Options:
  --verbose      Enable request logging`

server {
  port 8080
  session {
    secret env("SESSION_SECRET", "dev-secret")
  }
}

verbose = args["verbose"]
api_key = env("API_KEY")

before {
  if verbose {
    log_info(`${request.method} ${request.path}`)
  }
}

route GET "/health" {
  response.body = {status: "ok"}
}

route GET "/api/data" {
  key = request.headers["x-api-key"] ?? ""

  if key != api_key {
    response.status = 401
    response.body = {error: "Invalid API key"}
    return
  }

  response.body = {data: "Protected data"}
}
```

Run it:

```bash
./myapp -p 3000 --verbose
./myapp -e production.env
./myapp -e none -p 3000
```

### Database Connection

```httpdsl
db_type = args["db-type"] ?? env("DB_TYPE", "sqlite")
db_url = args["db"] ?? env("DATABASE_URL", "./app.db")

db_conn = db.open(db_type, db_url)

route GET "/stats" {
  count = db_conn.query_value("SELECT COUNT(*) FROM users", [])
  response.body = {user_count: count}
}
```

### Environment Detection

```httpdsl
mode = args["mode"] ?? env("MODE", "development")
is_production = mode == "production"

route GET "/" {
  response.body = {
    mode: mode,
    debug: !is_production
  }
}

error 500 {
  if is_production {
    response.body = {error: "Internal server error"}
  } else {
    response.body = {
      error: "Internal server error",
      path: request.path,
      method: request.method
    }
  }
}
```

### .env File

```env
MODE=development
DATABASE_URL=sqlite:./app.db
SESSION_SECRET=your-secret-key-here
API_KEY=sk_live_abc123
LOG_LEVEL=debug
FEATURE_BETA=true
```
