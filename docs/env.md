# Configuration

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
./myapp --port 8080 --mode production
```

```httpdsl
port = int(args["port"] ?? "3000")
mode = args["mode"] ?? "development"
```

Every `--key` consumes the next argument as its value. A `--key` at the end of the command line (with no following argument) is set to `true`:

```bash
./myapp --port 8080 --verbose
```

```httpdsl
if args["verbose"] {
  log_info("verbose is on")
}
```

> **Note:** `--key` always consumes the next argument as its value. `--foo --bar` sets `args["foo"]` to the string `"--bar"`, not `true`. Put value-less flags last.

## help Block

Define help text shown when the binary is run with `-h`:

```httpdsl
help `My API Server

A REST API for managing widgets.

Usage:
  ./myapp --port 8080 --db postgres://localhost/mydb

Options:
  --port <n>     Port to listen on (default: 3000)
  --db <url>     Database connection URL
  --verbose      Enable verbose logging`
```

The `help` keyword takes a backtick string. This text is printed before the built-in flags when `-h` is used.

## Built-in Flags

Every compiled binary supports these flags:

| Flag | Description |
|------|-------------|
| `-h` | Print help text (if defined) and list built-in flags, then exit |
| `-v` | Print `Built with httpdsl` and exit |
| `-e <path>` | Load a specific `.env` file instead of `.env` |
| `-e none` | Skip `.env` loading entirely |

Example output of `-h` with a help block:

```
My API Server

A REST API for managing widgets.

Flags:
  -e <path>   Load env file (default: .env, "none" to skip)
  -v          Show version
  -h          Show this help
```

## Load Order

Configuration sources from weakest to strongest:

1. **`.env` file** — loaded at startup, populates the `env()` map
2. **CLI `args`** — `--key value` flags, available via `args["key"]`

These are separate namespaces. `env()` reads from `.env`, `args` reads from CLI flags. Your code decides how to combine them:

```httpdsl
# CLI args override .env values
port = int(args["port"] ?? env("PORT", "3000"))
db_url = args["db"] ?? env("DATABASE_URL", "sqlite:./app.db")
```

## Server Block Limitations

Settings in the `server {}` block (port, templates, static, etc.) are parsed at **compile time** as literal values. You cannot use runtime expressions like `env()` in most settings.

The one exception is `session.secret`, which supports runtime expressions:

```httpdsl
server {
  port 3000
  templates "./templates"
  static "/assets" "./public"

  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")  # this works — evaluated at runtime
  }
}
```

## Examples

### API with CLI Configuration

```httpdsl
help `Widget API

Options:
  --port <n>     Port to listen on
  --verbose      Enable request logging`

server {
  port 3000
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
./myapp --port 8080 --verbose
./myapp -e production.env
./myapp -e none --port 3000
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
