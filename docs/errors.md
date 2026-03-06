# Error Handling

HTTPDSL provides try-catch blocks for error handling and throw statements for raising errors.

## Try-Catch

Handle errors gracefully:

```httpdsl
try {
  risky_operation()
} catch(err) {
  log_error(err)
}
```

## Throw Statement

Raise errors with messages:

```httpdsl
fn divide(a, b) {
  if b == 0 {
    throw "Division by zero"
  }
  return a / b
}

try {
  result = divide(10, 0)
} catch(err) {
  log_error(err)
}
```

Throw with error objects:

```httpdsl
fn get_user(id) {
  if id < 1 {
    throw {code: 400, message: "Invalid user ID"}
  }
  
  if id > 1000 {
    throw {code: 404, message: "User not found"}
  }
  
  return {id: id, name: "User " + str(id)}
}

try {
  user = get_user(9999)
} catch(err) {
  log_error(err.message)
}
```

## Route Error Handling

Handle errors in route handlers:

```httpdsl
route POST "/api/divide" json {
  a = request.data.a ?? 0
  b = request.data.b ?? 0
  
  try {
    if b == 0 {
      throw "Cannot divide by zero"
    }
    
    result = a / b
    
    response.body = {result: result}
  } catch(err) {
    response.status = 400
    response.body = {error: err}
  }
}
```

## Database Error Handling

```httpdsl
route GET "/api/users/:id" {
  user_id = int(request.params.id)
  
  try {
    conn = db.open("sqlite", "./data.db")
    user = conn.query_one("SELECT * FROM users WHERE id = ?", [user_id])
    conn.close()
    
    if user == null {
      throw {code: 404, message: "User not found"}
    }
    
    response.body = user
  } catch(err) {
    if type(err) == "object" {
      response.status = err.code
      response.body = {error: err.message}
    } else {
      response.status = 500
      response.body = {error: "Database error"}
    }
  }
}
```

## File Operation Errors

```httpdsl
route GET "/api/config" {
  try {
    config = file.read_json("./config.json")
    response.body = config
  } catch(err) {
    response.status = 500
    response.body = {error: "Failed to load configuration"}
    log_error(err)
  }
}
```

## Validation Errors

```httpdsl
fn validate_user(data) {
  if data.name == "" || data.name == null {
    throw {field: "name", message: "Name is required"}
  }
  
  if data.email == "" || data.email == null {
    throw {field: "email", message: "Email is required"}
  }
  
  if !is_email(data.email) {
    throw {field: "email", message: "Invalid email format"}
  }
  
  if data.age == null || data.age < 18 {
    throw {field: "age", message: "Must be 18 or older"}
  }
}

route POST "/api/users" json {
  try {
    validate_user(request.data)
    
    user = {
      id: cuid2(),
      name: request.data.name,
      email: request.data.email,
      age: request.data.age
    }
    
    response.status = 201
    response.body = user
  } catch(err) {
    response.status = 400
    response.body = {error: err}
  }
}
```

## Nested Try-Catch

```httpdsl
route POST "/api/process" json {
  try {
    data = request.data
    
    try {
      conn = db.open("sqlite", "./data.db")
      result = conn.exec("INSERT INTO items (name) VALUES (?)", [data.name])
      conn.close()
      
      response.body = {id: result.last_insert_id}
    } catch(db_err) {
      log_error("Database error: " + str(db_err))
      throw "Failed to save item"
    }
  } catch(err) {
    response.status = 500
    response.body = {error: err}
  }
}
```

## External API Errors

```httpdsl
route GET "/api/proxy" {
  url = request.query.url ?? ""
  
  if url == "" {
    response.status = 400
    response.body = {error: "URL parameter required"}
    return
  }
  
  try {
    result = fetch(url)
    
    if result.status >= 400 {
      throw `External API returned status ${result.status}`
    }
    
    response.body = result.body
  } catch(err) {
    response.status = 502
    response.body = {error: "Failed to fetch external resource", details: err}
  }
}
```

## JSON Parsing Errors

```httpdsl
route POST "/api/parse" text {
  json_string = request.data
  
  try {
    data = json.parse(json_string)
    response.body = {parsed: data}
  } catch(err) {
    response.status = 400
    response.body = {error: "Invalid JSON", details: err}
  }
}
```

## Authentication Errors

```httpdsl
fn verify_token(token) {
  if token == "" {
    throw {code: 401, message: "Missing authentication token"}
  }
  
  secret = env("JWT_SECRET")
  payload = jwt.verify(token, secret)
  
  if payload == null {
    throw {code: 401, message: "Invalid or expired token"}
  }
  
  return payload
}

route GET "/api/profile" {
  try {
    token = request.bearer
    payload = verify_token(token)
    
    response.body = {
      user_id: payload.user_id,
      email: payload.email
    }
  } catch(err) {
    if type(err) == "object" {
      response.status = err.code
      response.body = {error: err.message}
    } else {
      response.status = 500
      response.body = {error: "Internal error"}
    }
  }
}
```

## Custom Error Types

```httpdsl
fn validation_error(field, message) {
  return {
    type: "validation_error",
    field: field,
    message: message
  }
}

fn not_found_error(resource) {
  return {
    type: "not_found",
    resource: resource,
    message: `${resource} not found`
  }
}

fn unauthorized_error() {
  return {
    type: "unauthorized",
    message: "Authentication required"
  }
}

route POST "/api/posts" json {
  try {
    if !request.session.user_id {
      throw unauthorized_error()
    }
    
    title = request.data.title ?? ""
    
    if title == "" {
      throw validation_error("title", "Title is required")
    }
    
    post = {
      id: cuid2(),
      title: title,
      author_id: request.session.user_id,
      created_at: now()
    }
    
    response.status = 201
    response.body = post
  } catch(err) {
    switch err.type {
      case "validation_error" {
        response.status = 400
      }
      case "not_found" {
        response.status = 404
      }
      case "unauthorized" {
        response.status = 401
      }
      default {
        response.status = 500
      }
    }
    
    response.body = {error: err}
  }
}
```

## Error Recovery

```httpdsl
route GET "/api/data" {
  try {
    data = file.read_json("./data.json")
    response.body = data
  } catch(err) {
    log_warn("Failed to read data file, using defaults")
    
    response.body = {
      items: [],
      count: 0,
      message: "Using default data"
    }
  }
}
```
