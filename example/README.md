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
| **Env vars** | `env()` for configuration |
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
# Build
../httpdsl build taskboard.httpdsl

# Run
./taskboard
# → http://localhost:8000
```

Register an account, create tasks, move them between columns. Open a second
browser tab to see real-time SSE updates.

## File Structure

```
example/
├── taskboard.httpdsl          # Main application (single file)
├── templates/
│   ├── layout.gohtml          # Head/foot template defines
│   ├── login.gohtml           # Sign in page
│   ├── register.gohtml        # Registration page
│   ├── board.gohtml           # Main board page
│   └── partials/
│       └── task_card.gohtml   # (reserved for future template use)
└── static/
    └── css/
        └── app.css            # Complete stylesheet (~330 lines)
```

## CSS Highlights

- Custom properties for theming (change `--accent` to rebrand)
- Smooth animations: `card-in`, `card-out`, `slide-up`, `fade-in`, `toast-in/out`
- HTMX integration classes: `.htmx-added`, `.htmx-swapping`, `.htmx-settling`
- Responsive grid (3-column → 1-column on mobile)
- Hover-reveal action buttons on task cards
- Loading spinner for async operations
- Connection status dot (SSE alive indicator)
