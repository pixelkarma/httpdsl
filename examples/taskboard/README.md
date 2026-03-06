# Team Task Board — HTTPDSL Example

A real-time collaborative task board built entirely in HTTPDSL. Demonstrates
nearly every language feature in a single, practical application.

## Features Used

| Feature | Usage |
|---------|-------|
| **SQLite** | Users and tasks tables |
| **Templates** | Go html/template with `{{define}}` / `{{template}}` partials |
| **Sessions** | Login/logout, session-based auth |
| **CSRF** | Automatic protection on all POST/PATCH/DELETE |
| **Auth** | `hash_password()` / `verify_password()` for bcrypt passwords |
| **SSE** | Live updates — task create/move/delete broadcast to all clients |
| **Middleware** | Global `before` block enforces authentication |
| **Validation** | `validate()` for registration and task creation |
| **Store** | `store.sync()` with JSON file persistence |
| **Cron** | Nightly cleanup of old completed tasks |
| **Static files** | CSS served from `./static/css` |
| **Error pages** | Custom 404 and 500 |
| **Shutdown** | Graceful DB close on shutdown |
| **Functions** | Reusable card HTML builder, helpers |
| **Redirect** | `redirect()` + manual 302 for session-preserving redirects |
| **CLI args** | `args["db-path"]` with `??` fallback defaults |
| **Env file** | `env()` reads from `.env` file, not OS env |
| **Help block** | `help` with backtick string for `--help` output |
| **HTMX** | Partial HTML swaps for all interactions |

## Architecture

```
GET  /board       → Full page with all columns
POST /tasks       → Returns card HTML partial (appended to To Do)
PATCH /tasks/:id  → Moves card between columns (HX-Trigger event)
DELETE /tasks/:id → Removes card (outerHTML swap to empty)
SSE  /events      → Broadcasts create/move/delete to other clients
```

Every interaction after initial page load is an HTMX partial swap — no full
page reloads.

## Running

```bash
# From the repo root
cd examples/taskboard

# Build
../../httpdsl build app.httpdsl

# Run with defaults
./taskboard
# → http://localhost:8000

# Run with CLI args
./taskboard -p 3000 --db-path /tmp/mydb.db --secret my-production-key

# Run with a custom .env file
./taskboard -e production.env

# Show help
./taskboard -h
```

### CLI Options

```
Team Task Board — A real-time collaborative task board

Options:
  --db-path   Path to SQLite database (default: ./taskboard.db)
  --secret    Session secret key (default: change-me-in-production)
  --store     Path to store JSON file (default: ./taskboard_store.json)

Flags:
  -p <port>   Listen port (default: 8000, or PORT env var)
  -s <dir>    Static file directory (default: ./static/css)
  -e <path>   Load env file (default: .env, "none" to skip)
  -v          Show version
  -h          Show this help

Environment variables:
  PORT        Override listen port
  SSL_CERT    Path to TLS certificate file
  SSL_KEY     Path to TLS private key file
```

### Example .env File

```env
DB_PATH=./taskboard.db
SESSION_SECRET=change-me-in-production
STORE_PATH=./taskboard_store.json
```

Register an account, create tasks, move them between columns. Open a second
browser tab to see real-time SSE updates.

## File Structure

```
examples/taskboard/
├── app.httpdsl                # Main application (single file)
├── templates/
│   ├── layout.gohtml          # Head/foot template defines
│   ├── login.gohtml           # Sign in page
│   ├── register.gohtml        # Registration page
│   ├── board.gohtml           # Main board page
│   └── partials/
│       ├── error.gohtml       # Error page partial
│       └── task_card.gohtml   # Task card partial
└── static/
    └── css/
        ├── app.css            # Stylesheet (~330 lines)
        └── board.js           # SSE client with auto-reconnect
```

## CSS Highlights

- Custom properties for theming (change `--accent` to rebrand)
- Smooth animations: `card-in`, `card-out`, `slide-up`, `fade-in`, `toast-in/out`
- HTMX integration classes: `.htmx-added`, `.htmx-swapping`, `.htmx-settling`
- Responsive grid (3-column → 1-column on mobile)
- Hover-reveal action buttons on task cards
- Loading spinner for async operations
- Connection status dot (SSE alive indicator)
