# Middleware

Middleware functions run before or after route handlers, enabling cross-cutting concerns like logging, authentication, and response modification.

## Global Middleware

Runs on every request:

```httpdsl
server {
  port 3000
}

before {
  log_info(`${request.method} ${request.path}`)
}

after {
  log_info(`Response status: ${response.status}`)
}

route GET "/" {
  response.body = "Home"
}

route GET "/about" {
  response.body = "About"
}
```

## Before Middleware

Runs before the route handler:

```httpdsl
server {
  port 3000
}

before {
  request_id = cuid2()
  start_time = now()
  
  log_info(`[${request_id}] Request started`)
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## After Middleware

Runs after the route handler:

```httpdsl
server {
  port 3000
}

after {
  response.headers = {
    "X-Powered-By": "HTTPDSL",
    "X-Response-Time": str(now())
  }
}

route GET "/" {
  response.body = "Hello"
}
```

## Group Middleware

Scoped to specific route groups:

```httpdsl
server {
  port 3000
}

route GET "/" {
  response.body = "Public home"
}

group "/api" {
  before {
    log_info("API request")
  }
  
  after {
    response.headers = {"X-API-Version": "1.0"}
  }
  
  route GET "/users" {
    response.body = {users: []}
  }
  
  route GET "/posts" {
    response.body = {posts: []}
  }
}
```

## Execution Order

1. Global `before`
2. Group `before` (if route is in a group)
3. Route handler
4. Group `after` (if route is in a group)
5. Global `after`

```httpdsl
server {
  port 3000
}

before {
  log_info("1. Global before")
}

after {
  log_info("5. Global after")
}

group "/api" {
  before {
    log_info("2. Group before")
  }
  
  after {
    log_info("4. Group after")
  }
  
  route GET "/data" {
    log_info("3. Route handler")
    response.body = {data: "value"}
  }
}
```

## Authentication Middleware

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
  }
}

route POST "/login" json {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    request.session.user_id = 1
    request.session.username = username
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}

group "/protected" {
  before {
    if !request.session.user_id {
      response.status = 401
      response.body = {error: "Authentication required"}
      redirect("/login")
    }
  }
  
  route GET "/dashboard" {
    response.body = {
      message: "Welcome to dashboard",
      user: request.session.username
    }
  }
  
  route GET "/profile" {
    response.body = {
      user_id: request.session.user_id,
      username: request.session.username
    }
  }
}
```

## API Key Validation

```httpdsl
server {
  port 3000
}

api_key = env("API_KEY", "default-key")

route GET "/public" {
  response.body = {message: "Public endpoint"}
}

group "/api" {
  before {
    provided_key = request.headers["x-api-key"] ?? ""
    
    if provided_key == "" {
      response.status = 401
      response.body = {error: "API key required"}
      return
    }
    
    if provided_key != api_key {
      response.status = 401
      response.body = {error: "Invalid API key"}
      return
    }
  }
  
  route GET "/data" {
    response.body = {secret: "Protected data"}
  }
}
```

## JWT Authentication

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
      exp: now() + 3600
    }
    
    token = jwt.sign(payload, jwt_secret)
    
    response.body = {token: token}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}

group "/api" {
  before {
    token = request.bearer
    
    if token == "" {
      response.status = 401
      response.body = {error: "Missing authentication token"}
      return
    }
    
    payload = jwt.verify(token, jwt_secret)
    
    if payload == null {
      response.status = 401
      response.body = {error: "Invalid or expired token"}
      return
    }
    
    if payload.exp < now() {
      response.status = 401
      response.body = {error: "Token expired"}
      return
    }
  }
  
  route GET "/profile" {
    response.body = {
      user_id: payload.user_id,
      email: payload.email
    }
  }
}
```

## Request Logging

```httpdsl
server {
  port 3000
}

before {
  request_id = cuid2()
  start_time = now()
  
  log_info(`[${request_id}] ${request.method} ${request.path} from ${request.ip}`)
  
  user_agent = request.headers["user-agent"] ?? "Unknown"
  log_info(`[${request_id}] User-Agent: ${user_agent}`)
}

after {
  duration = now() - start_time
  
  log_info(`[${request_id}] Completed with status ${response.status} in ${duration}s`)
}

route GET "/api/users" {
  response.body = {users: []}
}

route POST "/api/users" json {
  response.status = 201
  response.body = {created: true}
}
```

## Response Headers

```httpdsl
server {
  port 3000
}

after {
  response.headers = {
    "X-Powered-By": "HTTPDSL",
    "X-Frame-Options": "DENY",
    "X-Content-Type-Options": "nosniff"
  }
}

route GET "/" {
  response.body = "Hello"
}
```

## CORS Headers

```httpdsl
server {
  port 3000
}

before {
  if request.method == "OPTIONS" {
    response.status = 204
    response.headers = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE",
      "Access-Control-Allow-Headers": "Content-Type, Authorization"
    }
    response.body = ""
    return
  }
}

after {
  response.headers = {
    "Access-Control-Allow-Origin": "*"
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Rate Limiting

```httpdsl
server {
  port 3000
}

request_counts = {}

before {
  client_ip = request.ip
  current_count = store.get(`rate:${client_ip}`, 0)
  
  if current_count >= 100 {
    response.status = 429
    response.body = {error: "Too many requests"}
    return
  }
  
  store.incr(`rate:${client_ip}`, 1, 60)
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Content Negotiation

```httpdsl
server {
  port 3000
}

before {
  accept = request.headers["accept"] ?? "*/*"
  
  if contains(accept, "application/json") {
    response.type = "json"
  } else if contains(accept, "text/html") {
    response.type = "html"
  } else if contains(accept, "text/plain") {
    response.type = "text"
  }
}

route GET "/data" {
  if response.type == "json" {
    response.body = {message: "Hello"}
  } else if response.type == "html" {
    response.body = "<h1>Hello</h1>"
  } else {
    response.body = "Hello"
  }
}
```

## Compression Headers

```httpdsl
server {
  port 3000
  gzip true
}

after {
  if response.status == 200 {
    response.headers = {
      "Cache-Control": "public, max-age=3600",
      "Vary": "Accept-Encoding"
    }
  }
}

route GET "/api/large-data" {
  response.body = {data: range(1000)}
}
```

## Error Recovery

```httpdsl
server {
  port 3000
}

before {
  try {
    token = request.bearer
    
    if token != "" {
      payload = jwt.verify(token, env("JWT_SECRET"))
      
      if payload != null {
        log_info(`Authenticated user: ${payload.user_id}`)
      }
    }
  } catch(err) {
    log_warn(`Authentication check failed: ${err}`)
  }
}

route GET "/" {
  response.body = "Hello"
}
```
