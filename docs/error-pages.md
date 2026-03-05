# Error Pages

Customize error responses for specific HTTP status codes.

## Basic Error Pages

Define custom handlers for error status codes:

```httpdsl
server {
  port 3000
}

error 404 {
  response.body = {
    error: "Page not found",
    path: request.path
  }
}

error 500 {
  response.body = {
    error: "Internal server error",
    message: "Something went wrong"
  }
}

route GET "/" {
  response.body = "Home"
}
```

## HTML Error Pages

```httpdsl
server {
  port 3000
}

error 404 {
  response.type = "html"
  response.body = `
    <!DOCTYPE html>
    <html>
      <head><title>404 Not Found</title></head>
      <body>
        <h1>404 - Page Not Found</h1>
        <p>The page <code>${request.path}</code> could not be found.</p>
        <a href="/">Go Home</a>
      </body>
    </html>
  `
}

error 500 {
  response.type = "html"
  response.body = `
    <!DOCTYPE html>
    <html>
      <head><title>500 Internal Server Error</title></head>
      <body>
        <h1>500 - Internal Server Error</h1>
        <p>An unexpected error occurred. Please try again later.</p>
      </body>
    </html>
  `
}

route GET "/" {
  response.body = "Home"
}
```

## Access Request Data

Error handlers have access to the `request` object:

```httpdsl
server {
  port 3000
}

error 404 {
  log_warn(`404 Not Found: ${request.method} ${request.path} from ${request.ip}`)
  
  response.body = {
    error: "Not found",
    method: request.method,
    path: request.path,
    timestamp: date()
  }
}

route GET "/" {
  response.body = "Home"
}
```

## Multiple Error Codes

```httpdsl
server {
  port 3000
}

error 400 {
  response.body = {
    error: "Bad request",
    message: "The request could not be understood"
  }
}

error 401 {
  response.body = {
    error: "Unauthorized",
    message: "Authentication required"
  }
}

error 403 {
  response.body = {
    error: "Forbidden",
    message: "You don't have permission to access this resource"
  }
}

error 404 {
  response.body = {
    error: "Not found",
    path: request.path
  }
}

error 500 {
  response.body = {
    error: "Internal server error",
    request_id: cuid2()
  }
}

route GET "/" {
  response.body = "Home"
}
```

## Template-Based Error Pages

```httpdsl
server {
  port 3000
  templates "./templates"
}

error 404 {
  render("error.gohtml", {
    status: 404,
    title: "Page Not Found",
    message: `The page ${request.path} was not found`,
    show_home_link: true
  })
}

error 500 {
  render("error.gohtml", {
    status: 500,
    title: "Server Error",
    message: "An unexpected error occurred",
    show_home_link: true
  })
}

route GET "/" {
  response.body = "Home"
}
```

## Conditional Error Responses

```httpdsl
server {
  port 3000
}

error 404 {
  accept = request.headers["accept"] ?? "*/*"
  
  if contains(accept, "application/json") {
    response.type = "json"
    response.body = {
      error: "Not found",
      path: request.path
    }
  } else {
    response.type = "html"
    response.body = `
      <!DOCTYPE html>
      <html>
        <head><title>404 Not Found</title></head>
        <body>
          <h1>404 - Page Not Found</h1>
          <p>Could not find: ${request.path}</p>
        </body>
      </html>
    `
  }
}

route GET "/api/users" {
  response.body = {users: []}
}

route GET "/" {
  response.body = "Home"
}
```

## Error Logging

```httpdsl
server {
  port 3000
}

error 404 {
  log_warn(`404: ${request.method} ${request.path} from ${request.ip}`)
  
  response.body = {
    error: "Not found",
    path: request.path
  }
}

error 500 {
  log_error(`500: ${request.method} ${request.path} from ${request.ip}`)
  
  error_id = cuid2()
  
  log_error(`Error ID: ${error_id}`)
  
  response.body = {
    error: "Internal server error",
    error_id: error_id,
    message: "Please contact support with this error ID"
  }
}

route GET "/" {
  response.body = "Home"
}
```

## Development vs Production

```httpdsl
server {
  port 3000
}

is_production = env("ENV") == "production"

error 500 {
  if is_production {
    response.body = {
      error: "Internal server error",
      request_id: cuid2()
    }
  } else {
    response.body = {
      error: "Internal server error",
      path: request.path,
      method: request.method,
      headers: request.headers,
      timestamp: date()
    }
  }
}

route GET "/" {
  response.body = "Home"
}
```

## API Error Format

```httpdsl
server {
  port 3000
}

error 400 {
  response.body = {
    success: false,
    error: {
      code: "BAD_REQUEST",
      message: "Invalid request parameters",
      status: 400
    }
  }
}

error 401 {
  response.body = {
    success: false,
    error: {
      code: "UNAUTHORIZED",
      message: "Authentication required",
      status: 401
    }
  }
}

error 403 {
  response.body = {
    success: false,
    error: {
      code: "FORBIDDEN",
      message: "Insufficient permissions",
      status: 403
    }
  }
}

error 404 {
  response.body = {
    success: false,
    error: {
      code: "NOT_FOUND",
      message: "Resource not found",
      path: request.path,
      status: 404
    }
  }
}

error 500 {
  response.body = {
    success: false,
    error: {
      code: "INTERNAL_ERROR",
      message: "An unexpected error occurred",
      status: 500,
      request_id: cuid2()
    }
  }
}

route GET "/api/users" {
  response.body = {
    success: true,
    data: {users: []}
  }
}
```

## Custom Headers in Errors

```httpdsl
server {
  port 3000
}

error 401 {
  response.headers = {
    "WWW-Authenticate": 'Bearer realm="API"'
  }
  response.body = {
    error: "Authentication required"
  }
}

error 404 {
  response.headers = {
    "X-Error-Code": "NOT_FOUND",
    "X-Request-ID": cuid2()
  }
  response.body = {
    error: "Not found"
  }
}

error 429 {
  reset_time = date("unix") + 60
  
  response.headers = {
    "Retry-After": "60",
    "X-RateLimit-Reset": str(reset_time)
  }
  response.body = {
    error: "Too many requests",
    retry_after: 60
  }
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Redirect on Error

```httpdsl
server {
  port 3000
}

error 404 {
  if starts_with(request.path, "/old/") {
    new_path = replace(request.path, "/old/", "/new/")
    redirect(new_path)
  } else {
    response.body = {
      error: "Not found",
      path: request.path
    }
  }
}

route GET "/new/page" {
  response.body = "New page location"
}
```
