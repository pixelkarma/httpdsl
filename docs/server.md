# Server Configuration

The `server` block configures your HTTP server. All configuration is optional with sensible defaults.

## Basic Configuration

```httpdsl
server {
  port 3000
}
```

## All Options

```httpdsl
server {
  port 8080
  gzip true
  throttle_requests_per_second 100
  static "/assets" "./public"
  templates "./templates"
  
  cors {
    origins "*"
    methods "GET,POST,PUT,DELETE"
    headers "Content-Type, Authorization"
  }
  
  session {
    cookie "sid"
    expires 1 h
    secret "your-secret-key-here"
    csrf true
    csrf_safe_origins ["https://example.com"]
  }
}
```

## Port

Specify which port the server listens on:

```httpdsl
server {
  port 3000
}
```

Default: `8080`

## Gzip Compression

Enable automatic gzip compression for responses:

```httpdsl
server {
  port 3000
  gzip true
}
```

Default: `false`

## Rate Limiting

Throttle incoming requests per second:

```httpdsl
server {
  port 3000
  throttle_requests_per_second 100
}
```

Exceeds this limit will receive a 429 Too Many Requests response.

## Static Files

Serve static files from a directory:

```httpdsl
server {
  port 3000
  static "/assets" "./public"
}
```

Files in `./public` will be served under `/assets`. For example:
- `./public/logo.png` → `/assets/logo.png`
- `./public/css/style.css` → `/assets/css/style.css`

## Templates

Specify the directory containing Go HTML templates:

```httpdsl
server {
  port 3000
  templates "./templates"
}

route GET "/" {
  render("index.gohtml", {title: "Home"})
}
```

See [Templates](templates.md) for details.

## CORS

Configure Cross-Origin Resource Sharing:

```httpdsl
server {
  port 3000
  
  cors {
    origins "https://example.com,https://app.example.com"
    methods "GET,POST,PUT,DELETE"
    headers "Content-Type, Authorization, X-API-Key"
  }
}
```

Or allow all origins:

```httpdsl
server {
  port 3000
  
  cors {
    origins "*"
    methods "GET,POST"
    headers "Content-Type"
  }
}
```

The server automatically handles OPTIONS preflight requests.

See [CORS](cors.md) for details.

## Sessions

Enable session management:

```httpdsl
server {
  port 3000
  
  session {
    cookie "session_id"
    expires 24 h
    secret "change-this-secret-key"
  }
}
```

### Session Options

- `cookie`: Cookie name (default: `"sid"`)
- `expires`: Session duration (e.g., `1 h`, `30 m`, `7 d`)
- `secret`: Secret key for signing sessions (required for production)
- `csrf`: Enable CSRF protection (default: `false`)
- `csrf_safe_origins`: Trusted origins that bypass CSRF for cross-origin requests

See [Sessions](sessions.md) and [CSRF](csrf.md) for details.

## Runtime Configuration

Most `server {}` settings are **compile-time literals** — you cannot use `env()` or `args` in them. The exception is `session.secret`, which supports runtime expressions:

```httpdsl
server {
  port 3000
  
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET", "dev-secret")
  }
}
```

For runtime configuration, use CLI args and `.env` files in your `init` block. See [Configuration](env.md) for details.

## Complete Example

```httpdsl
server {
  port 3000
  gzip true
  throttle_requests_per_second 100
  static "/public" "./static"
  templates "./views"
  
  cors {
    origins "*"
    methods "GET,POST,PUT,DELETE,PATCH"
    headers "Content-Type, Authorization"
  }
  
  session {
    cookie "sid"
    expires 7 d
    secret env("SESSION_SECRET", "dev-secret")
    csrf true
    csrf_safe_origins ["https://trusted.example.com"]
  }
}

route GET "/" {
  response.body = "Server configured!"
}
```
