# Groups

Groups organize routes with a common path prefix and shared middleware.

## Basic Groups

Prefix routes with a common path:

```httpdsl
server {
  port 3000
}

group "/api" {
  route GET "/users" {
    response.body = {users: []}
  }
  
  route GET "/posts" {
    response.body = {posts: []}
  }
}
```

The routes become:
- `/api/users`
- `/api/posts`

## Nested Groups

Groups can be nested:

```httpdsl
server {
  port 3000
}

group "/api" {
  group "/v1" {
    route GET "/users" {
      response.body = {version: "v1", users: []}
    }
  }
  
  group "/v2" {
    route GET "/users" {
      response.body = {version: "v2", users: []}
    }
  }
}
```

The routes become:
- `/api/v1/users`
- `/api/v2/users`

## Group Middleware

Add `before` and `after` blocks to groups:

```httpdsl
server {
  port 3000
}

group "/api" {
  before {
    log_info(`API request: ${request.method} ${request.path}`)
  }
  
  after {
    log_info("API request completed")
  }
  
  route GET "/users" {
    response.body = {users: []}
  }
  
  route GET "/posts" {
    response.body = {posts: []}
  }
}
```

Middleware runs for all routes in the group.

## Authentication Group

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
  }
}

route GET "/" {
  response.body = "Public home page"
}

route POST "/login" json {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    session.user_id = 1
    session.username = username
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}

group "/admin" {
  before {
    if !session.user_id {
      response.status = 401
      response.body = {error: "Authentication required"}
      redirect("/login")
    }
  }
  
  route GET "/dashboard" {
    response.body = {
      message: "Admin dashboard",
      user: session.username
    }
  }
  
  route GET "/users" {
    response.body = {
      users: [{id: 1, name: "Alice"}]
    }
  }
  
  route DELETE "/users/:id" {
    user_id = request.params.id
    response.body = {deleted: user_id}
  }
}
```

## API Versioning

```httpdsl
server {
  port 3000
}

group "/api/v1" {
  before {
    response.headers = {"X-API-Version": "1.0"}
  }
  
  route GET "/users" {
    response.body = {version: 1, users: []}
  }
}

group "/api/v2" {
  before {
    response.headers = {"X-API-Version": "2.0"}
  }
  
  route GET "/users" {
    response.body = {
      version: 2,
      users: [],
      pagination: {page: 1, total: 0}
    }
  }
}
```

## API Key Authentication

```httpdsl
server {
  port 3000
}

api_key = env("API_KEY", "secret-key")

route GET "/public" {
  response.body = {message: "Public endpoint"}
}

group "/api" {
  before {
    key = request.headers["x-api-key"] ?? ""
    
    if key != api_key {
      response.status = 401
      response.body = {error: "Invalid API key"}
      return
    }
  }
  
  after {
    log_info(`API call to ${request.path} completed`)
  }
  
  route GET "/data" {
    response.body = {secret: "Protected data"}
  }
  
  route POST "/data" json {
    response.body = {created: true}
  }
}
```

## Role-Based Access Control

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
  }
}

fn require_role(role) {
  user_role = session.role ?? "guest"
  
  if user_role != role {
    response.status = 403
    response.body = {error: "Insufficient permissions"}
    return false
  }
  
  return true
}

group "/admin" {
  before {
    if !require_role("admin") {
      return
    }
  }
  
  route GET "/dashboard" {
    response.body = {message: "Admin dashboard"}
  }
  
  route DELETE "/users/:id" {
    response.body = {deleted: request.params.id}
  }
}

group "/moderator" {
  before {
    user_role = session.role ?? "guest"
    
    if user_role != "moderator" && user_role != "admin" {
      response.status = 403
      response.body = {error: "Insufficient permissions"}
      return
    }
  }
  
  route GET "/reports" {
    response.body = {reports: []}
  }
  
  route POST "/ban/:id" {
    response.body = {banned: request.params.id}
  }
}
```

## Request Logging

```httpdsl
server {
  port 3000
}

group "/api" {
  before {
    request_id = cuid2()
    start_time = date("unix")
    
    log_info(`[${request_id}] ${request.method} ${request.path} from ${request.ip}`)
  }
  
  after {
    duration = date("unix") - start_time
    log_info(`[${request_id}] Completed in ${duration}s with status ${response.status}`)
  }
  
  route GET "/users" {
    sleep(100)
    response.body = {users: []}
  }
  
  route POST "/users" json {
    sleep(200)
    response.status = 201
    response.body = {created: true}
  }
}
```

## CORS for API Group

```httpdsl
server {
  port 3000
}

group "/api" {
  before {
    origin = request.headers["origin"] ?? ""
    
    allowed_origins = [
      "https://example.com",
      "https://app.example.com"
    ]
    
    is_allowed = false
    
    each allowed in allowed_origins {
      if origin == allowed {
        is_allowed = true
        break
      }
    }
    
    if is_allowed {
      response.headers = {
        "Access-Control-Allow-Origin": origin,
        "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE",
        "Access-Control-Allow-Headers": "Content-Type, Authorization"
      }
    }
  }
  
  route GET "/data" {
    response.body = {data: "Value"}
  }
}
```

## Multiple Groups

```httpdsl
server {
  port 3000
}

route GET "/" {
  response.body = "Home"
}

group "/api" {
  before {
    response.headers = {"Content-Type": "application/json"}
  }
  
  route GET "/status" {
    response.body = {status: "ok"}
  }
}

group "/admin" {
  before {
    if !session.admin {
      response.status = 403
      response.body = {error: "Admin access required"}
      return
    }
  }
  
  route GET "/dashboard" {
    response.body = {admin: true}
  }
}

group "/public" {
  after {
    response.headers = {"Cache-Control": "public, max-age=3600"}
  }
  
  route GET "/about" {
    response.body = "About us"
  }
}
```

## Middleware Execution Order

For a request to `/api/users`:

1. Global `before` (if any)
2. Group `before`
3. Route handler
4. Group `after`
5. Global `after` (if any)

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
  
  route GET "/users" {
    log_info("3. Route handler")
    response.body = {users: []}
  }
}
```
