# The Compiler

HTTPDSL compiles `.httpdsl` source files into standalone native Go binaries. There is no interpreter, no runtime, no framework dependency. The output is a single static executable.

## Prerequisites

- **Go 1.24+** — required both to build the compiler and to compile generated code
- Git (to clone the repository)

## Building from Source

```bash
git clone <repo-url>
cd httpdsl
go build -o httpdsl .
```

This produces the `httpdsl` compiler binary. Move it to your PATH:

```bash
sudo mv httpdsl /usr/local/bin/
```

## Usage

### `httpdsl build`

Compile a `.httpdsl` file into a native binary:

```bash
# Single file
httpdsl build server.httpdsl
# Produces: ./server

# Auto-detect (looks for app.httpdsl in current directory)
httpdsl build
```

### `httpdsl run`

Compile, execute, and **watch for changes** — the development workflow:

```bash
httpdsl run server.httpdsl
```

This compiles to a temp directory, starts the server, and watches all files in the project directory recursively. When any file changes (`.httpdsl`, `.gohtml`, `.css`, etc.), it automatically rebuilds and restarts the server.

```
  httpdsl v0.1.0

  ➜  Server:   http://localhost:8080/
  ➜  Built in: 606ms
  ➜  Watching: ./

  [watch] 2 files changed:
    modified  app.httpdsl
    modified  templates/index.gohtml

  ➜  Rebuilding...
  ➜  Server:   http://localhost:8080/
  ➜  Built in: 547ms
```

- Changes are debounced (500ms) so bulk writes don't trigger multiple rebuilds
- The running server receives SIGTERM before restart (triggers your `shutdown {}` block)
- Build errors are displayed inline — the watcher stays active and rebuilds on the next save
- Hidden directories (`.git`), `node_modules`, and `vendor` are excluded from watching
- Ctrl+C cleanly shuts down both the watcher and the server

## Compilation Model

The pipeline has three stages:

```
.httpdsl source  →  Go source code  →  native binary
   (parse)           (generate)         (go build)
```

1. **Parse** — The compiler lexes and parses your `.httpdsl` files into an AST. Syntax errors are reported with line numbers.
2. **Generate** — The AST is compiled into a single `main.go` file in a temp directory, alongside a `go.mod` with auto-detected dependencies.
3. **Build** — `go build -ldflags="-s -w" -o <binary> .` with `CGO_ENABLED=0` produces a statically-linked binary.

The temp directory is cleaned up after compilation. If the build fails, the generated source is saved to `/tmp/<name>.go` for debugging.

## Output Naming

| Input | Output |
|---|---|
| `server.httpdsl` | `./server` |
| `api.httpdsl` | `./api` |
| `app.httpdsl` in `myproject/` | `./myproject` |
| Directory with `app.httpdsl` | Named after directory |

## Project Mode

For larger applications, use a directory with `app.httpdsl` as the entry point:

```
myproject/
├── app.httpdsl          # server {} block + shared config
├── routes/
│   ├── users.httpdsl    # user routes
│   └── products.httpdsl # product routes
└── middleware/
    └── auth.httpdsl     # before/after hooks
```

```bash
cd myproject
httpdsl build
# Finds app.httpdsl, recursively includes all .httpdsl files
# Produces: ./myproject
```

`app.httpdsl` is always parsed first (it should contain the `server {}` block). All other `.httpdsl` files in the directory tree are included automatically.

## Dependency Auto-Detection

The compiler inspects your code and adds the necessary Go modules:

| Feature Used | Dependency Added |
|---|---|
| `db.open("sqlite", ...)` | `modernc.org/sqlite` (pure Go) |
| `db.open("postgres", ...)` | `github.com/jackc/pgx/v5` |
| `db.open("mysql", ...)` | `github.com/go-sql-driver/mysql` |
| `db.open("mongo", ...)` | `go.mongodb.org/mongo-driver/v2` |
| `hash_password()` / `verify_password()` | `golang.org/x/crypto` |

No manual dependency management required.

## Static Binaries

All binaries are compiled with `CGO_ENABLED=0`, producing fully static executables:

- No libc dependency
- No dynamic linking
- Copy to any machine with the same OS/architecture and run
- Perfect for `FROM scratch` Docker images

## Deployment

### Direct

```bash
httpdsl build app.httpdsl
scp app user@server:/opt/myapp/
ssh user@server '/opt/myapp/app'
```

### Systemd

```ini
[Unit]
Description=My HTTPDSL App
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/myapp
ExecStart=/opt/myapp/app
Restart=always
Environment=DATABASE_URL=postgres://localhost/mydb

[Install]
WantedBy=multi-user.target
```

### Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o httpdsl . && ./httpdsl build app.httpdsl

FROM scratch
COPY --from=builder /build/app /app
EXPOSE 3000
CMD ["/app"]
```

## Next Steps

- [Hello World](hello-world.md) — your first server
- [Server Configuration](server.md) — the `server {}` block
