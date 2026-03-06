# Init Block

The `init` block runs once at startup, after the `.env` file is loaded but before the server starts accepting requests. Variables assigned in `init` become **globals** — accessible from every route, middleware, scheduled task, and shutdown handler.

## Basic Usage

```httpdsl
init {
  app_name = "My App"
  debug = env("DEBUG", "false") == "true"
  log_info(`${app_name} starting (debug=${debug})`)
}

route GET "/" {
  response.body = {name: app_name, debug: debug}
}
```

`app_name` and `debug` are available everywhere because they were assigned in `init`.

## Global Variables

Any variable assigned in `init` (or at the top level) is emitted as a package-level Go variable. This means:

- **Routes** can read and write them
- **Middleware** (`before`/`after`) can read and write them
- **Scheduled tasks** (`every`) can read and write them
- **Shutdown handlers** can read them
- **Functions** can read and write them

```httpdsl
init {
  counter = 0
  db = db.open("sqlite", "./app.db")
}

route POST "/increment" {
  counter += 1
  response.body = {count: counter}
}

route GET "/count" {
  response.body = {count: counter}
}

shutdown {
  log_info(`Final count: ${counter}`)
  db.close()
}
```

> **Note:** Global variables are shared across all goroutines. Simple reads and writes to a single variable are safe in Go, but complex concurrent mutations may need care. For shared state, consider using `store` which is concurrency-safe.

## Execution Order

The startup sequence is:

1. **CLI flags** parsed (`-p`, `-s`, `-e`, `-v`, `-h`)
2. **`.env` file** loaded (populates `env()` map)
3. **`init` blocks** run (in source order)
4. **Templates** initialized
5. **Routes** registered
6. **Server** starts listening

This means `init` has access to `env()` values and `args`, but the server is not yet accepting requests.

## Common Patterns

### Database Setup

```httpdsl
init {
  db_url = args["db"] ?? env("DATABASE_URL", "sqlite:./app.db")
  db = db.open("sqlite", db_url)

  db.exec(`
    CREATE TABLE IF NOT EXISTS users (
      id       INTEGER PRIMARY KEY AUTOINCREMENT,
      name     TEXT NOT NULL,
      email    TEXT UNIQUE NOT NULL,
      created  TEXT DEFAULT (datetime('now'))
    )
  `, [])

  log_info("Database ready")
}
```

### Configuration from CLI and .env

```httpdsl
init {
  mode = args["mode"] ?? env("MODE", "development")
  is_production = mode == "production"
  api_key = env("API_KEY")

  if api_key == null && is_production {
    log_error("API_KEY required in production")
  }

  log_info(`Running in ${mode} mode`)
}

before {
  if is_production {
    log_info(`${request.method} ${request.path}`)
  }
}
```

### Store Initialization

```httpdsl
init {
  store.sync("./data.json", 10)
  store.set("started_at", now())
  store.set("request_count", 0)
}

before {
  store.incr("request_count", 1)
}

route GET "/stats" {
  response.body = {
    started: store.get("started_at"),
    requests: store.get("request_count")
  }
}
```

### Session Store with Database

```httpdsl
server {
  port 8080
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET", "dev-secret")
  }
}

init {
  db = db.open("sqlite", "./app.db")
  set_session_store(db, "sessions", 5)
}
```

## Top-Level Statements

Statements written at the top level (outside any block) are treated as implicit init code:

```httpdsl
server { port 8080 }

# These are equivalent to being inside init { }
app_name = "My App"
db = db.open("sqlite", "./app.db")
log_info("Ready")

route GET "/" {
  response.body = {name: app_name}
}
```

This is equivalent to:

```httpdsl
server { port 8080 }

init {
  app_name = "My App"
  db = db.open("sqlite", "./app.db")
  log_info("Ready")
}

route GET "/" {
  response.body = {name: app_name}
}
```

Using an explicit `init` block is recommended for clarity, especially in larger files.

## Multiple Init Blocks

You can have multiple `init` blocks. They run in source order:

```httpdsl
init {
  db = db.open("sqlite", "./app.db")
  log_info("Database connected")
}

init {
  db.exec("CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY, msg TEXT)", [])
  log_info("Schema ready")
}
```

This is useful for organizing setup into logical sections, but a single `init` block is more common.

## Variables Not in Init

Variables assigned inside routes, middleware, or functions are **local** — they don't persist across requests:

```httpdsl
route GET "/example" {
  x = 42          # local to this request
  response.body = {x: x}
}

route GET "/other" {
  # x is NOT available here
  response.body = {x: null}
}
```

To share state across routes, assign the variable in `init`:

```httpdsl
init {
  x = 42           # global — available everywhere
}

route POST "/update" {
  x = int(request.body.value)
  response.body = {x: x}
}

route GET "/read" {
  response.body = {x: x}  # sees the updated value
}
```

## Complete Example

```httpdsl
help `Task API

Options:
  --db <path>    Database file (default: ./tasks.db)
  --verbose      Enable request logging`

server {
  port 8080
  gzip true
  session {
    cookie "sid"
    expires 7 d
    secret env("SESSION_SECRET", "dev-secret")
  }
}

init {
  db_path = args["db"] ?? env("DB_PATH", "./tasks.db")
  verbose = args["verbose"] ?? false
  db = db.open("sqlite", db_path)

  db.exec(`
    CREATE TABLE IF NOT EXISTS tasks (
      id      INTEGER PRIMARY KEY AUTOINCREMENT,
      title   TEXT NOT NULL,
      done    INTEGER DEFAULT 0,
      created TEXT DEFAULT (datetime('now'))
    )
  `, [])

  store.sync("./app_store.json", 10)
  store.set("started_at", now())

  log_info(`Task API ready (db: ${db_path})`)
}

before {
  if verbose {
    log_info(`${request.method} ${request.path}`)
  }
}

route GET "/tasks" {
  tasks = db.query("SELECT * FROM tasks ORDER BY created DESC", [])
  response.body = tasks
}

route POST "/tasks" {
  db.exec("INSERT INTO tasks (title) VALUES (?)", [request.body.title])
  response.status = 201
  response.body = {created: true}
}

shutdown {
  log_info("Closing database")
  db.close()
}
```
