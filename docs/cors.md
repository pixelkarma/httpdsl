# CORS

Cross-Origin Resource Sharing (CORS) allows your API to be accessed from different domains.

## Server-Level CORS

Configure CORS in the server block:

```httpdsl
server {
  port 3000
  
  cors {
    origins "*"
    methods "GET,POST,PUT,DELETE"
    headers "Content-Type, Authorization"
  }
}

route GET "/api/data" {
  response.body = {data: "Available cross-origin"}
}
```

## Specific Origins

Allow specific domains:

```httpdsl
server {
  port 3000
  
  cors {
    origins "https://example.com,https://app.example.com"
    methods "GET,POST"
    headers "Content-Type"
  }
}
```

## Allow All Origins

```httpdsl
server {
  port 3000
  
  cors {
    origins "*"
    methods "GET,POST,PUT,DELETE,PATCH"
    headers "Content-Type, Authorization, X-Requested-With"
  }
}
```

## OPTIONS Preflight

The server automatically handles OPTIONS preflight requests when CORS is configured.

## Manual CORS Headers

Set CORS headers manually:

```httpdsl
server {
  port 3000
}

before {
  response.headers = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE",
    "Access-Control-Allow-Headers": "Content-Type, Authorization"
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Conditional CORS

Allow specific origins dynamically:

```httpdsl
server {
  port 3000
}

allowed_origins = [
  "https://example.com",
  "https://app.example.com",
  "https://mobile.example.com"
]

before {
  origin = request.headers["origin"] ?? ""
  
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
      "Access-Control-Allow-Headers": "Content-Type, Authorization",
      "Access-Control-Allow-Credentials": "true"
    }
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## CORS with Credentials

```httpdsl
server {
  port 3000
}

before {
  origin = request.headers["origin"] ?? ""
  
  if origin == "https://example.com" {
    response.headers = {
      "Access-Control-Allow-Origin": origin,
      "Access-Control-Allow-Methods": "GET, POST",
      "Access-Control-Allow-Headers": "Content-Type",
      "Access-Control-Allow-Credentials": "true"
    }
  }
}

route GET "/api/user" {
  response.body = {user: "data"}
}
```

**Note**: When using credentials, `Access-Control-Allow-Origin` cannot be `*`.

## OPTIONS Handler

Manual OPTIONS preflight:

```httpdsl
server {
  port 3000
}

before {
  if request.method == "OPTIONS" {
    response.status = 204
    response.headers = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH",
      "Access-Control-Allow-Headers": "Content-Type, Authorization, X-API-Key",
      "Access-Control-Max-Age": "86400"
    }
    response.body = ""
    return
  }
  
  response.headers = {
    "Access-Control-Allow-Origin": "*"
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}

route POST "/api/data" json {
  response.body = {received: request.data}
}
```

## Group-Specific CORS

```httpdsl
server {
  port 3000
}

route GET "/public" {
  response.body = "Public data"
}

group "/api" {
  before {
    response.headers = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE",
      "Access-Control-Allow-Headers": "Content-Type, Authorization"
    }
    
    if request.method == "OPTIONS" {
      response.status = 204
      response.body = ""
      return
    }
  }
  
  route GET "/users" {
    response.body = {users: []}
  }
  
  route POST "/users" json {
    response.status = 201
    response.body = {created: true}
  }
}
```

## Environment-Based CORS

```httpdsl
server {
  port 3000
}

is_production = env("ENV") == "production"
allowed_origin = is_production ? "https://example.com" : "*"

before {
  response.headers = {
    "Access-Control-Allow-Origin": allowed_origin,
    "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE",
    "Access-Control-Allow-Headers": "Content-Type, Authorization"
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Regex Origin Matching

```httpdsl
server {
  port 3000
}

before {
  origin = request.headers["origin"] ?? ""
  
  if origin != "" && (starts_with(origin, "https://") && ends_with(origin, ".example.com")) {
    response.headers = {
      "Access-Control-Allow-Origin": origin,
      "Access-Control-Allow-Methods": "GET, POST",
      "Access-Control-Allow-Headers": "Content-Type",
      "Access-Control-Allow-Credentials": "true"
    }
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Complete CORS Example

```httpdsl
server {
  port 3000
}

allowed_origins = [
  "https://example.com",
  "https://app.example.com"
]

fn is_origin_allowed(origin) {
  each allowed in allowed_origins {
    if origin == allowed {
      return true
    }
  }
  return false
}

before {
  origin = request.headers["origin"] ?? ""
  
  if origin != "" && is_origin_allowed(origin) {
    response.headers = {
      "Access-Control-Allow-Origin": origin,
      "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH",
      "Access-Control-Allow-Headers": "Content-Type, Authorization, X-Requested-With",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Max-Age": "86400"
    }
  }
  
  if request.method == "OPTIONS" {
    response.status = 204
    response.body = ""
    return
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}

route POST "/api/data" json {
  response.body = {received: request.data}
}

route PUT "/api/data/:id" json {
  response.body = {updated: request.params.id}
}

route DELETE "/api/data/:id" {
  response.body = {deleted: request.params.id}
}
```

## CORS with Authentication

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
  }
}

before {
  origin = request.headers["origin"] ?? ""
  
  if origin == "https://app.example.com" {
    response.headers = {
      "Access-Control-Allow-Origin": origin,
      "Access-Control-Allow-Methods": "GET, POST",
      "Access-Control-Allow-Headers": "Content-Type, Authorization",
      "Access-Control-Allow-Credentials": "true"
    }
  }
  
  if request.method == "OPTIONS" {
    response.status = 204
    response.body = ""
    return
  }
}

route POST "/auth/login" json {
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

route GET "/api/profile" {
  if !session.user_id {
    response.status = 401
    response.body = {error: "Not authenticated"}
    return
  }
  
  response.body = {
    user_id: session.user_id,
    username: session.username
  }
}
```

## Exposed Headers

```httpdsl
server {
  port 3000
}

before {
  response.headers = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Expose-Headers": "X-Total-Count, X-Page"
  }
}

route GET "/api/items" {
  items = range(100)
  
  response.headers = {
    "X-Total-Count": str(len(items)),
    "X-Page": "1"
  }
  
  response.body = {items: slice(items, 0, 10)}
}
```
