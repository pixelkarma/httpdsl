# Environment Variables

Access environment variables with the `env()` function.

## Basic Usage

```httpdsl
port = env("PORT")
log_info(port)
```

## Default Values

Provide a default if the variable is not set:

```httpdsl
port = env("PORT", "3000")
db_url = env("DATABASE_URL", "sqlite::memory:")
log_level = env("LOG_LEVEL", "info")
```

## Server Configuration

```httpdsl
server {
  port int(env("PORT", "3000"))
  gzip env("GZIP_ENABLED", "true") == "true"
  
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")
  }
}
```

## Database Connection

```httpdsl
db_type = env("DB_TYPE", "sqlite")
db_url = env("DATABASE_URL", "./app.db")

db_conn = db.open(db_type, db_url)

route GET "/stats" {
  count = db_conn.query_value("SELECT COUNT(*) FROM users", [])
  response.body = {user_count: count}
}
```

## API Keys

```httpdsl
server {
  port 3000
}

api_key = env("API_KEY")

route GET "/api/data" {
  provided_key = request.headers["x-api-key"] ?? ""
  
  if provided_key != api_key {
    response.status = 401
    response.body = {error: "Invalid API key"}
    return
  }
  
  response.body = {data: "Protected data"}
}
```

## JWT Secret

```httpdsl
server {
  port 3000
}

jwt_secret = env("JWT_SECRET")

route POST "/auth/login" json {
  {email, password} = request.data
  
  if email == "user@example.com" && password == "password" {
    payload = {
      user_id: 1,
      email: email,
      exp: date("unix") + 3600
    }
    
    token = jwt.sign(payload, jwt_secret)
    
    response.body = {token: token}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}

route GET "/api/profile" {
  token = request.bearer
  
  if token == "" {
    response.status = 401
    response.body = {error: "Missing token"}
    return
  }
  
  payload = jwt.verify(token, jwt_secret)
  
  if payload == null {
    response.status = 401
    response.body = {error: "Invalid token"}
    return
  }
  
  response.body = {user_id: payload.user_id}
}
```

## Environment Detection

```httpdsl
server {
  port 3000
}

is_production = env("ENV") == "production"
is_development = env("ENV", "development") == "development"

route GET "/" {
  response.body = {
    environment: env("ENV", "development"),
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
      method: request.method,
      timestamp: date()
    }
  }
}
```

## External Services

```httpdsl
server {
  port 3000
}

stripe_key = env("STRIPE_SECRET_KEY")
sendgrid_key = env("SENDGRID_API_KEY")
aws_key = env("AWS_ACCESS_KEY_ID")
aws_secret = env("AWS_SECRET_ACCESS_KEY")

route POST "/payment" json {
  amount = request.data.amount
  
  result = fetch("https://api.stripe.com/v1/charges", {
    method: "POST",
    headers: {
      "Authorization": `Bearer ${stripe_key}`,
      "Content-Type": "application/x-www-form-urlencoded"
    },
    body: `amount=${amount}&currency=usd`
  })
  
  response.body = result.body
}
```

## CORS Configuration

```httpdsl
server {
  port 3000
}

allowed_origins = split(env("CORS_ORIGINS", "*"), ",")

before {
  origin = request.headers["origin"] ?? ""
  
  is_allowed = false
  
  each allowed in allowed_origins {
    if trim(allowed) == "*" || trim(allowed) == origin {
      is_allowed = true
      break
    }
  }
  
  if is_allowed {
    response.headers = {
      "Access-Control-Allow-Origin": origin == "" ? "*" : origin,
      "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE"
    }
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Feature Flags

```httpdsl
server {
  port 3000
}

feature_beta = env("FEATURE_BETA", "false") == "true"
feature_new_ui = env("FEATURE_NEW_UI", "false") == "true"

route GET "/features" {
  response.body = {
    beta: feature_beta,
    new_ui: feature_new_ui
  }
}

route GET "/" {
  if feature_new_ui {
    response.body = "New UI"
  } else {
    response.body = "Classic UI"
  }
}
```

## Rate Limits

```httpdsl
server {
  port 3000
}

max_requests = int(env("RATE_LIMIT", "100"))
rate_window = int(env("RATE_WINDOW", "60"))

before {
  client_ip = request.ip
  key = `rate:${client_ip}`
  
  count = store.get(key, 0)
  
  if count >= max_requests {
    response.status = 429
    response.body = {error: "Rate limit exceeded"}
    return
  }
  
  store.incr(key, 1, rate_window)
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Complete Example

```httpdsl
server {
  port int(env("PORT", "3000"))
  gzip env("GZIP", "true") == "true"
  
  session {
    cookie env("SESSION_COOKIE", "sid")
    expires 24 h
    secret env("SESSION_SECRET")
    csrf env("CSRF_ENABLED", "true") == "true"
  }
}

is_production = env("ENV") == "production"
log_level = env("LOG_LEVEL", "info")

db_conn = db.open(
  env("DB_TYPE", "sqlite"),
  env("DATABASE_URL", "./app.db")
)

jwt_secret = env("JWT_SECRET")
api_key = env("API_KEY")

before {
  if log_level == "debug" {
    log_info(`${request.method} ${request.path} from ${request.ip}`)
  }
}

route GET "/" {
  response.body = {
    app: env("APP_NAME", "HTTPDSL App"),
    version: env("VERSION", "1.0.0"),
    environment: env("ENV", "development")
  }
}

route GET "/health" {
  response.body = {
    status: "ok",
    timestamp: date()
  }
}

error 500 {
  error_id = cuid2()
  
  log_error(`Error ${error_id}: ${request.method} ${request.path}`)
  
  if is_production {
    response.body = {
      error: "Internal server error",
      error_id: error_id
    }
  } else {
    response.body = {
      error: "Internal server error",
      error_id: error_id,
      path: request.path,
      method: request.method,
      debug: true
    }
  }
}
```

## .env File Example

Create a `.env` file:

```env
PORT=3000
ENV=development
DATABASE_URL=sqlite:./app.db
SESSION_SECRET=your-secret-key-here
JWT_SECRET=jwt-secret-key
API_KEY=api-key-here
LOG_LEVEL=debug
GZIP=true
CSRF_ENABLED=true
```

Load with environment loader or export:

```bash
export $(cat .env | xargs)
httpdsl run server.httpdsl
```
