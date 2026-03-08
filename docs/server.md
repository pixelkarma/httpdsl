# Server Configuration

The `server` block configures your HTTP server. All configuration is optional with sensible defaults.

## Basic Configuration

```httpdsl
server {
  port 8080
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
  ssl_cert "/path/to/cert.pem"
  ssl_key "/path/to/key.pem"
  autocert "yourdomain.com"
  autocert_dir "/var/lib/httpdsl/certs"
  
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
  port 8080
}
```

Default: `8080`

## SSL / TLS

Enable HTTPS by providing paths to a certificate and private key:

```httpdsl
server {
  port 443
  ssl_cert "/etc/ssl/certs/mydomain.pem"
  ssl_key "/etc/ssl/private/mydomain-key.pem"
}
```

Both `ssl_cert` and `ssl_key` must be set. The server will use Go's `ListenAndServeTLS` which supports TLS 1.2+ with modern cipher suites by default.

These settings accept string literals or runtime expressions like `env()`:

```httpdsl
server {
  port 443
  ssl_cert env("SSL_CERT", "/etc/ssl/cert.pem")
  ssl_key env("SSL_KEY", "/etc/ssl/key.pem")
}
```

You can also skip these entirely and enable TLS at runtime with `SSL_CERT`/`SSL_KEY` environment variables — see [Runtime Overrides](#runtime-overrides).

This works with any PEM-encoded certificate — self-signed, CA-issued, or Let's Encrypt.

### Self-Signed Certificate (Development)

Generate a self-signed cert for local development:

```bash
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes -subj '/CN=localhost'
```

Then reference the files:

```httpdsl
server {
  port 8443
  ssl_cert "./cert.pem"
  ssl_key "./key.pem"
}
```

### Let's Encrypt (Autocert)

Built-in automatic TLS via Let's Encrypt. Certificates are provisioned and renewed automatically — no certbot, no cron jobs.

```httpdsl
server {
  port 443
  autocert "yourdomain.com"
  autocert_dir "/var/lib/httpdsl/certs"
}
```

When enabled:
- On first HTTPS request, a certificate is automatically obtained from Let's Encrypt
- Certificates are cached to `autocert_dir` (default: `.autocert`)
- An HTTP server on port 80 handles ACME challenges and redirects all other traffic to HTTPS
- Certificates auto-renew before expiry (~30 days before the 90-day expiration)

Both settings accept expressions:

```httpdsl
server {
  port 443
  autocert env("AUTOCERT_DOMAIN")
  autocert_dir env("AUTOCERT_DIR", "/var/lib/httpdsl/certs")
}
```

Or skip the server block entirely and use CLI flags or environment variables:

```bash
# CLI flags
./myapp -a yourdomain.com -ad /var/lib/certs

# Environment variables
AUTOCERT_DOMAIN=yourdomain.com AUTOCERT_DIR=/var/lib/certs ./myapp
```

> **Note:** Autocert requires ports 80 and 443 to be accessible from the internet. The server must be reachable at the configured domain.

## Gzip Compression

Enable automatic gzip compression for responses:

```httpdsl
server {
  port 8080
  gzip true
}
```

Default: `false`

## Rate Limiting

Throttle incoming requests per second:

```httpdsl
server {
  port 8080
  throttle_requests_per_second 100
}
```

Exceeds this limit will receive a 429 Too Many Requests response.

## Static Files

Serve static files from a directory:

```httpdsl
server {
  port 8080
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
  port 8080
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
  port 8080
  
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
  port 8080
  
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
  port 8080
  
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

## Runtime Overrides

Compiled binaries accept flags and environment variables that override `server {}` settings at runtime.

### Port

Precedence: `-p` flag → `PORT` env var → compiled default.

```bash
# Use compiled default (8080)
./myapp

# Override with flag
./myapp -p 3000

# Override with env var
PORT=3000 ./myapp

# Flag wins over env var
PORT=3000 ./myapp -p 4000   # listens on 4000
```

### SSL / TLS (Runtime)

Precedence: `SSL_CERT`/`SSL_KEY` env vars → compiled default → no TLS.

```bash
# Enable TLS at runtime (no ssl_cert/ssl_key in server block needed)
SSL_CERT=/path/to/cert.pem SSL_KEY=/path/to/key.pem ./myapp

# Override compiled-in cert paths
SSL_CERT=/new/cert.pem SSL_KEY=/new/key.pem ./myapp
```

The `ssl_cert` and `ssl_key` settings also support runtime expressions:

```httpdsl
server {
  port 443
  ssl_cert env("MY_CERT", "/etc/ssl/cert.pem")
  ssl_key env("MY_KEY", "/etc/ssl/key.pem")
}
```

### Static Directory

Precedence: `-s` flag → compiled default.

```bash
# Use compiled default
./myapp

# Override the primary static directory
./myapp -s /var/www/static
```

The `-s` flag overrides the first `static` mount's directory.

### Autocert (Runtime)

Precedence: `-a` flag → `AUTOCERT_DOMAIN` env var → compiled default.

```bash
# Enable autocert at runtime (no autocert in server block needed)
./myapp -a yourdomain.com

# With custom cache dir
./myapp -a yourdomain.com -ad /var/lib/certs

# Via environment variables
AUTOCERT_DOMAIN=yourdomain.com AUTOCERT_DIR=/var/lib/certs ./myapp
```

Autocert takes priority over `ssl_cert`/`ssl_key` when both are configured.

### All Flags

```
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
```

Run `./myapp -h` to see the defaults from your `server {}` block.

### Other Runtime Configuration

Most `server {}` settings are **compile-time literals** — you cannot use `env()` or `args` in them. The exceptions are `session.secret`, `ssl_cert`, `ssl_key`, `autocert`, and `autocert_dir`, which support runtime expressions:

```httpdsl
server {
  port 8080
  
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET", "dev-secret")
  }
}
```

For other runtime configuration, use CLI args and `.env` files in your `init` block. See [Configuration](env.md) for details.

## Complete Example

```httpdsl
server {
  port 443
  gzip true
  throttle_requests_per_second 100
  static "/public" "./static"
  templates "./views"
  ssl_cert "/etc/ssl/certs/mydomain.pem"
  ssl_key "/etc/ssl/private/mydomain-key.pem"
  
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
