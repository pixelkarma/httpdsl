# Request Object

- [Properties](#properties)
  - [method](#method)
  - [path](#path)
  - [params](#params)
  - [query](#query)
  - [data](#data)
  - [headers](#headers)
  - [cookies](#cookies)
  - [ip](#ip)
  - [bearer](#bearer)
  - [basic](#basic)
- [Complete Example](#complete-example)
- [Validation Example](#validation-example)

The `request` object provides access to incoming HTTP request data.

## Properties

### method

HTTP method as uppercase string:

```httpdsl
route GET "/debug" {
  response.body = {method: request.method}
}
```

Returns: `"GET"`, `"POST"`, `"PUT"`, `"PATCH"`, `"DELETE"`

### path

Request path:

```httpdsl
route GET "/api/*path" {
  response.body = {path: request.path}
}
```

Example: `/api/users` returns `"/api/users"`

### params

Hash of path parameters:

```httpdsl
route GET "/users/:id/posts/:post_id" {
  user_id = request.params.id
  post_id = request.params.post_id
  
  response.body = {
    user_id: user_id,
    post_id: post_id
  }
}
```

Wildcard paths:

```httpdsl
route GET "/files/*path" {
  file_path = request.params.path
  response.body = {file: file_path}
}
```

### query

Hash of query string parameters:

```httpdsl
route GET "/search" {
  query = request.query.q ?? ""
  page = int(request.query.page ?? "1")
  limit = int(request.query.limit ?? "10")
  
  response.body = {
    query: query,
    page: page,
    limit: limit
  }
}
```

Example: `/search?q=httpdsl&page=2&limit=20`

All query values are strings. Convert as needed:

```httpdsl
route GET "/items" {
  max_price = float(request.query.max_price ?? "0")
  available = request.query.available == "true"
  
  response.body = {
    max_price: max_price,
    available: available
  }
}
```

### data

Parsed request body. Type depends on Content-Type:

**JSON bodies** (application/json):

```httpdsl
route POST "/api/users" json {
  name = request.data.name
  email = request.data.email
  age = request.data.age
  
  response.body = {
    received: {name: name, email: email, age: age}
  }
}
```

**Form data** (application/x-www-form-urlencoded, multipart/form-data):

```httpdsl
route POST "/submit" form {
  username = request.data.username
  password = request.data.password
  
  response.body = {username: username}
}
```

**Plain text**:

```httpdsl
route POST "/echo" text {
  content = request.data
  response.body = content
}
```

### headers

Hash of request headers (lowercase keys):

```httpdsl
route GET "/debug" {
  user_agent = request.headers["user-agent"] ?? "Unknown"
  content_type = request.headers["content-type"] ?? "None"
  custom_header = request.headers["x-custom-header"] ?? "Not set"
  
  response.body = {
    user_agent: user_agent,
    content_type: content_type,
    custom: custom_header
  }
}
```

All header names are lowercase:

```httpdsl
route GET "/api/data" {
  api_key = request.headers["x-api-key"] ?? ""
  
  if api_key != env("API_KEY") {
    response.status = 401
    response.body = {error: "Invalid API key"}
    return
  }
  
  response.body = {data: "Secret data"}
}
```

### cookies

Hash of cookies:

```httpdsl
route GET "/check" {
  session_id = request.cookies.session_id ?? ""
  preferences = request.cookies.preferences ?? "default"
  
  response.body = {
    session_id: session_id,
    preferences: preferences
  }
}
```

### ip

Client IP address:

```httpdsl
route GET "/" {
  client_ip = request.ip
  log_info(`Request from ${client_ip}`)
  response.body = {ip: client_ip}
}
```

### bearer

Bearer token from Authorization header:

```httpdsl
route GET "/api/protected" {
  token = request.bearer
  
  if token == "" {
    response.status = 401
    response.body = {error: "Missing token"}
    return
  }
  
  secret = env("JWT_SECRET")
  try {
    payload = jwt.verify(token, secret)
  } catch(err) {
    response.status = 401
    response.body = {error: "Invalid token"}
    return
  }
  
  response.body = {user_id: payload.user_id}
}
```

Returns empty string if no Bearer token is present.

### basic

Basic authentication credentials:

```httpdsl
route GET "/protected" {
  auth = request.basic
  
  if auth == null {
    response.status = 401
    response.headers = {"WWW-Authenticate": 'Basic realm="Restricted"'}
    response.body = {error: "Authentication required"}
    return
  }
  
  if auth.username != "admin" || auth.password != "secret" {
    response.status = 401
    response.body = {error: "Invalid credentials"}
    return
  }
  
  response.body = {message: "Access granted"}
}
```

Returns `null` if no Basic auth is present, otherwise returns:

```httpdsl
{
  username: "user",
  password: "pass"
}
```

## Complete Example

```httpdsl
server {
  port 3000
}

route POST "/api/posts/:id/comments" json {
  post_id = int(request.params.id)
  page = int(request.query.page ?? "1")
  
  {author, content} = request.data
  
  token = request.bearer
  user_agent = request.headers["user-agent"] ?? "Unknown"
  client_ip = request.ip
  
  log_info(`New comment from ${client_ip} using ${user_agent}`)
  
  if token == "" {
    response.status = 401
    response.body = {error: "Authentication required"}
    return
  }
  
  if author == "" || content == "" {
    response.status = 400
    response.body = {error: "Missing required fields"}
    return
  }
  
  comment = {
    id: cuid2(),
    post_id: post_id,
    author: author,
    content: content,
    ip: client_ip,
    created_at: now()
  }
  
  response.status = 201
  response.body = comment
}
```

## Validation Example

Use a `before` block for cross-cutting request validation:

```httpdsl
init {
  api_key = env("API_KEY")
}

before {
  key = request.headers["x-api-key"] ?? ""
  
  if key != api_key {
    response.status = 401
    response.body = {error: "Invalid API key"}
    return
  }
}

route POST "/api/data" json {
  response.body = {success: true, data: request.data}
}
```
