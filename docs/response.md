# Response Object

The `response` object controls the HTTP response sent to clients.

## Properties

### body

Set response body. Type determines Content-Type:

**Hash or Array** → JSON:

```httpdsl
route GET "/api/users" {
  response.body = [
    {id: 1, name: "Alice"},
    {id: 2, name: "Bob"}
  ]
}
```

```httpdsl
route GET "/api/user/:id" {
  response.body = {
    id: 1,
    name: "Alice",
    email: "alice@example.com"
  }
}
```

**String** → Text:

```httpdsl
route GET "/" {
  response.body = "Hello, World!"
}
```

### status

HTTP status code (default: 200):

```httpdsl
route POST "/api/users" json {
  response.status = 201
  response.body = {id: cuid2(), name: request.data.name}
}
```

Common status codes:

```httpdsl
route GET "/api/items/:id" {
  item_id = request.params.id
  
  if item_id == "1" {
    response.status = 200
    response.body = {id: item_id, name: "Item 1"}
  } else {
    response.status = 404
    response.body = {error: "Item not found"}
  }
}
```

### type

Content-Type. Values: `"json"`, `"text"`, `"html"`:

```httpdsl
route GET "/page" {
  response.type = "html"
  response.body = "<h1>Hello</h1>"
}
```

```httpdsl
route GET "/data" {
  response.type = "json"
  response.body = {message: "Hello"}
}
```

```httpdsl
route GET "/plain" {
  response.type = "text"
  response.body = "Plain text response"
}
```

**Note**: When `response.body` is a hash or array, type is automatically set to `"json"`. Explicit type setting is rarely needed.

### headers

Set custom response headers:

```httpdsl
route GET "/" {
  response.headers = {
    "X-Custom-Header": "value",
    "X-API-Version": "1.0"
  }
  response.body = {message: "Hello"}
}
```

Cache control:

```httpdsl
route GET "/api/data" {
  response.headers = {
    "Cache-Control": "public, max-age=3600"
  }
  response.body = {data: "Cacheable"}
}
```

CORS headers (if not using server-level CORS):

```httpdsl
route GET "/api/public" {
  response.headers = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST"
  }
  response.body = {public: true}
}
```

### cookies

Set response cookies:

```httpdsl
route POST "/login" json {
  response.cookies = {
    session_id: {
      value: cuid2(),
      path: "/",
      httponly: true,
      secure: true,
      maxage: 3600,
      samesite: "Strict"
    }
  }
  response.body = {success: true}
}
```

Cookie options:

- `value`: Cookie value (required)
- `path`: Cookie path (default: `/`)
- `httponly`: HttpOnly flag (default: `false`)
- `secure`: Secure flag (default: `false`)
- `maxage`: Max age in seconds
- `samesite`: SameSite attribute (`"Strict"`, `"Lax"`, or `"None"`)

Simple cookie:

```httpdsl
route GET "/set-theme" {
  theme = request.query.theme ?? "light"
  
  response.cookies = {
    theme: {
      value: theme,
      maxage: 86400
    }
  }
  
  response.body = {theme: theme}
}
```

Multiple cookies:

```httpdsl
route POST "/preferences" json {
  response.cookies = {
    lang: {
      value: request.data.lang,
      maxage: 31536000
    },
    theme: {
      value: request.data.theme,
      maxage: 31536000
    }
  }
  
  response.body = {saved: true}
}
```

Delete cookie (set maxage to -1):

```httpdsl
route POST "/logout" {
  response.cookies = {
    session_id: {
      value: "",
      maxage: -1
    }
  }
  response.body = {logged_out: true}
}
```

## Complete Examples

### REST API with Status Codes

```httpdsl
server {
  port 3000
}

users = [
  {id: 1, name: "Alice", email: "alice@example.com"},
  {id: 2, name: "Bob", email: "bob@example.com"}
]

route GET "/api/users" {
  response.body = users
}

route GET "/api/users/:id" {
  user_id = int(request.params.id)
  found = null
  
  each user in users {
    if user.id == user_id {
      found = user
      break
    }
  }
  
  if found == null {
    response.status = 404
    response.body = {error: "User not found"}
  } else {
    response.status = 200
    response.body = found
  }
}

route POST "/api/users" json {
  {name, email} = request.data
  
  if name == "" || email == "" {
    response.status = 400
    response.body = {error: "Name and email are required"}
    return
  }
  
  new_user = {
    id: len(users) + 1,
    name: name,
    email: email
  }
  
  users = append(users, new_user)
  
  response.status = 201
  response.headers = {
    "Location": `/api/users/${new_user.id}`
  }
  response.body = new_user
}

route DELETE "/api/users/:id" {
  user_id = int(request.params.id)
  new_users = []
  deleted = false
  
  each user in users {
    if user.id == user_id {
      deleted = true
    } else {
      new_users = append(new_users, user)
    }
  }
  
  if !deleted {
    response.status = 404
    response.body = {error: "User not found"}
  } else {
    users = new_users
    response.status = 204
    response.body = ""
  }
}
```

### HTML Response

```httpdsl
route GET "/page" {
  response.type = "html"
  response.body = `
    <!DOCTYPE html>
    <html>
      <head><title>HTTPDSL Page</title></head>
      <body>
        <h1>Welcome to HTTPDSL</h1>
        <p>This is a dynamically generated page.</p>
      </body>
    </html>
  `
}
```

### File Download

```httpdsl
route GET "/download/:filename" {
  filename = request.params.filename
  
  if !file.exists(`./downloads/${filename}`) {
    response.status = 404
    response.body = {error: "File not found"}
    return
  }
  
  content = file.read(`./downloads/${filename}`)
  
  response.headers = {
    "Content-Disposition": `attachment; filename="${filename}"`,
    "Content-Type": "application/octet-stream"
  }
  response.body = content
}
```

### Conditional Response

```httpdsl
route GET "/api/data" {
  format = request.query.format ?? "json"
  
  data = {message: "Hello", timestamp: date()}
  
  if format == "json" {
    response.type = "json"
    response.body = data
  } else if format == "text" {
    response.type = "text"
    response.body = `Message: ${data.message}, Time: ${data.timestamp}`
  } else {
    response.status = 400
    response.body = {error: "Invalid format. Use json or text"}
  }
}
```

### API with Rate Limit Headers

```httpdsl
route GET "/api/limited" {
  remaining = 100
  limit = 100
  reset = date("unix") + 3600
  
  response.headers = {
    "X-RateLimit-Limit": str(limit),
    "X-RateLimit-Remaining": str(remaining),
    "X-RateLimit-Reset": str(reset)
  }
  
  response.body = {data: "Your data here"}
}
```

### Redirect

```httpdsl
route GET "/old-page" {
  response.status = 301
  response.headers = {
    "Location": "/new-page"
  }
  response.body = ""
}
```

Or use the `redirect()` builtin:

```httpdsl
route GET "/old-page" {
  redirect("/new-page")
}

route GET "/permanent" {
  redirect("/new-location", 301)
}
```
