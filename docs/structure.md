# File Structure

- [Quick Example](#quick-example)
- [All Top-Level Blocks](#all-top-level-blocks)
- [Execution Order](#execution-order)
- [Globals vs Locals](#globals-vs-locals)

An `.httpdsl` file is a series of **top-level blocks**. There are no loose statements — everything lives inside a block.

## Quick Example

Every top-level block type in one file:

```httpdsl
help `User API

Options:
  --verbose   Enable request logging`

server {
  port 8080
  gzip true
  static "/assets" "./public"
}

init {
  db = db.open("sqlite", "./app.db")
  verbose = args["verbose"] ?? false
}

fn format_user(user) {
  return {id: user.id, name: upper(user.name)}
}

before {
  if verbose { log_info(`${request.method} ${request.path}`) }
}

after {
  response.headers["X-Powered-By"] = "httpdsl"
}

route GET "/" {
  response.body = {status: "ok"}
}

group "/api/users" {
  route GET "/" {
    users = db.query("SELECT * FROM users", [])
    response.body = map(users, fn(u) { return format_user(u) })
  }

  route GET "/:id" {
    user = db.query_one("SELECT * FROM users WHERE id = ?", [request.params.id])
    response.body = format_user(user)
  }
}

every 1 h {
  db.exec("DELETE FROM sessions WHERE expires < datetime('now')", [])
}

error 404 {
  response.body = {error: "not found"}
}

shutdown {
  db.close()
}
```

## All Top-Level Blocks

| Block | Purpose | Docs |
|-------|---------|------|
| `server { }` | Port, TLS, gzip, static files, sessions, CORS | [Server](server.md) |
| `init { }` | Startup code — variables assigned here are **global** | [Init](init.md) |
| `fn name() { }` | Reusable functions | [Functions](functions.md) |
| `route METHOD "/path" { }` | HTTP endpoint handlers | [Routes](routes.md) |
| `group "/prefix" { }` | Route collections with shared middleware | [Groups](groups.md) |
| `before { }` | Runs before every request | [Middleware](middleware.md) |
| `after { }` | Runs after every request | [Middleware](middleware.md) |
| `error <code> { }` | Custom error pages (404, 500, etc.) | [Error Pages](error-pages.md) |
| `every <interval> { }` | Scheduled tasks (interval or cron) | [Scheduling](scheduling.md) |
| `shutdown { }` | Cleanup on SIGINT/SIGTERM | [Shutdown](shutdown.md) |
| `help \`text\`` | Help text shown with `-h` flag | [Configuration](env.md) |

Nothing else is allowed at the top level. Bare statements like `x = 1` outside a block will produce a compile error.

## Execution Order

1. **CLI flags** parsed (`-p`, `-s`, `-e`, `-v`, `-h`)
2. **`.env` file** loaded
3. **`init`** blocks run (in source order) — globals are set here
4. **Templates** compiled
5. **Routes** registered, **middleware** attached
6. **Server** starts listening
7. **`every`** tasks begin running
8. On signal → **`shutdown`** blocks run

## Globals vs Locals

Variables assigned in `init` are global — visible in every route, middleware block, scheduled task, and function:

```httpdsl
init {
  app_name = "MyApp"   # global
}

route GET "/" {
  x = 42               # local to this request
  response.body = {name: app_name, x: x}
}

route GET "/other" {
  # app_name is available, x is not
  response.body = {name: app_name}
}
```

See [Init](init.md) for details.
