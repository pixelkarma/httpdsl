# Authentication

HTTPDSL provides built-in functions for password hashing and JWT token generation.

## Password Hashing

### hash_password()

Hash passwords using bcrypt (default) or argon2:

```httpdsl
route POST "/register" json {
  {email, password} = request.data
  
  hashed = hash_password(password)
  
  response.body = {
    email: email,
    password_hash: hashed
  }
}
```

With argon2:

```httpdsl
hashed = hash_password(password, "argon2")
```

With custom bcrypt cost:

```httpdsl
hashed = hash_password(password, "bcrypt", {cost: 14})
```

### verify_password()

Verify passwords (auto-detects algorithm):

```httpdsl
route POST "/login" json {
  {email, password} = request.data
  
  stored_hash = "$2a$10$..."
  
  if verify_password(password, stored_hash) {
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```

## JWT Tokens

### jwt.sign()

Create JWT tokens:

```httpdsl
secret = env("JWT_SECRET")

route POST "/auth/login" json {
  {email, password} = request.data
  
  if email == "user@example.com" && password == "password" {
    payload = {
      user_id: 1,
      email: email,
      exp: now() + 3600
    }
    
    token = jwt.sign(payload, secret)
    
    response.body = {token: token}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```

With custom algorithm:

```httpdsl
token = jwt.sign(payload, secret, "HS384")
token = jwt.sign(payload, secret, "HS512")
```

Default is HS256.

### jwt.verify()

Verify and decode JWT tokens:

```httpdsl
secret = env("JWT_SECRET")

route GET "/api/profile" {
  token = request.bearer
  
  if token == "" {
    response.status = 401
    response.body = {error: "Missing token"}
    return
  }
  
  try {
    payload = jwt.verify(token, secret)
  } catch(err) {
    response.status = 401
    response.body = {error: "Invalid token"}
    return
  }
  
  response.body = {
    user_id: payload.user_id,
    email: payload.email
  }
}
```

> **Note:** `jwt.verify()` throws on invalid or expired tokens. Always wrap in `try/catch`.

## Complete Auth System

```httpdsl
server {
  port 3000
}

jwt_secret = env("JWT_SECRET", "dev-secret-key")

db_conn = db.open("sqlite", "./auth.db")

db_conn.exec(`
  CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    email TEXT UNIQUE,
    password_hash TEXT,
    created_at TEXT
  )
`, [])

route POST "/auth/register" json {
  {email, password} = request.data
  
  if email == "" || password == "" {
    response.status = 400
    response.body = {error: "Email and password required"}
    return
  }
  
  if len(password) < 8 {
    response.status = 400
    response.body = {error: "Password must be at least 8 characters"}
    return
  }
  
  if !is_email(email) {
    response.status = 400
    response.body = {error: "Invalid email format"}
    return
  }
  
  existing = db_conn.query_one("SELECT id FROM users WHERE email = ?", [email])
  
  if existing != null {
    response.status = 409
    response.body = {error: "Email already registered"}
    return
  }
  
  hashed = hash_password(password)
  
  result = db_conn.exec(
    "INSERT INTO users (email, password_hash, created_at) VALUES (?, ?, ?)",
    [email, hashed, now()]
  )
  
  response.status = 201
  response.body = {
    id: result.last_insert_id,
    email: email
  }
}

route POST "/auth/login" json {
  {email, password} = request.data
  
  if email == "" || password == "" {
    response.status = 400
    response.body = {error: "Email and password required"}
    return
  }
  
  user = db_conn.query_one(
    "SELECT id, email, password_hash FROM users WHERE email = ?",
    [email]
  )
  
  if user == null {
    response.status = 401
    response.body = {error: "Invalid credentials"}
    return
  }
  
  if !verify_password(password, user.password_hash) {
    response.status = 401
    response.body = {error: "Invalid credentials"}
    return
  }
  
  payload = {
    user_id: user.id,
    email: user.email,
    exp: now() + 86400
  }
  
  token = jwt.sign(payload, jwt_secret)
  
  response.body = {
    token: token,
    user: {
      id: user.id,
      email: user.email
    }
  }
}

fn get_current_user() {
  token = request.bearer
  
  if token == "" {
    return null
  }
  
  try {
    payload = jwt.verify(token, jwt_secret)
  } catch(err) {
    return null
  }
  
  return payload
}

route GET "/auth/me" {
  user = get_current_user()
  
  if user == null {
    response.status = 401
    response.body = {error: "Unauthorized"}
    return
  }
  
  response.body = {
    user_id: user.user_id,
    email: user.email
  }
}

group "/api" {
  before {
    user = get_current_user()
    
    if user == null {
      response.status = 401
      response.body = {error: "Authentication required"}
      return
    }
  }
  
  route GET "/dashboard" {
    response.body = {
      message: "Welcome to dashboard",
      user_id: user.user_id
    }
  }
  
  route GET "/profile" {
    profile = db_conn.query_one(
      "SELECT id, email, created_at FROM users WHERE id = ?",
      [user.user_id]
    )
    
    response.body = profile
  }
}
```

## Session-Based Auth

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")
  }
}

db_conn = db.open("sqlite", "./users.db")

db_conn.exec(`
  CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    username TEXT UNIQUE,
    password_hash TEXT
  )
`, [])

route POST "/register" json {
  {username, password} = request.data
  
  if len(password) < 8 {
    response.status = 400
    response.body = {error: "Password too short"}
    return
  }
  
  hashed = hash_password(password)
  
  try {
    result = db_conn.exec(
      "INSERT INTO users (username, password_hash) VALUES (?, ?)",
      [username, hashed]
    )
    
    response.status = 201
    response.body = {id: result.last_insert_id}
  } catch(err) {
    response.status = 409
    response.body = {error: "Username already exists"}
  }
}

route POST "/login" json {
  {username, password} = request.data
  
  user = db_conn.query_one(
    "SELECT id, username, password_hash FROM users WHERE username = ?",
    [username]
  )
  
  if user == null || !verify_password(password, user.password_hash) {
    response.status = 401
    response.body = {error: "Invalid credentials"}
    return
  }
  
  request.session.user_id = user.id
  request.session.username = user.username
  
  response.body = {success: true}
}

route POST "/logout" {
  request.session.destroy()
  response.body = {success: true}
}

route GET "/protected" {
  if !request.session.user_id {
    response.status = 401
    response.body = {error: "Not authenticated"}
    return
  }
  
  response.body = {
    message: "Protected resource",
    user: request.session.username
  }
}
```

## Basic Authentication

```httpdsl
server {
  port 3000
}

route GET "/admin" {
  auth = request.basic
  
  if auth == null {
    response.status = 401
    response.headers = {
      "WWW-Authenticate": 'Basic realm="Admin Area"'
    }
    response.body = {error: "Authentication required"}
    return
  }
  
  if auth.username != "admin" || auth.password != "secret" {
    response.status = 401
    response.body = {error: "Invalid credentials"}
    return
  }
  
  response.body = {message: "Welcome, admin!"}
}
```

## API Key Authentication

```httpdsl
server {
  port 3000
}

api_keys = {
  "key123": {user_id: 1, name: "App 1"},
  "key456": {user_id: 2, name: "App 2"}
}

route GET "/api/data" {
  api_key = request.headers["x-api-key"] ?? ""
  
  if api_key == "" {
    response.status = 401
    response.body = {error: "API key required"}
    return
  }
  
  key_info = api_keys[api_key] ?? null
  
  if key_info == null {
    response.status = 401
    response.body = {error: "Invalid API key"}
    return
  }
  
  response.body = {
    data: "Secret data",
    client: key_info.name
  }
}
```

## OAuth2-Style Token

```httpdsl
server {
  port 3000
}

jwt_secret = env("JWT_SECRET")

route POST "/oauth/token" json {
  {grant_type, username, password} = request.data
  
  if grant_type != "password" {
    response.status = 400
    response.body = {error: "unsupported_grant_type"}
    return
  }
  
  if username != "user" || password != "pass" {
    response.status = 401
    response.body = {error: "invalid_grant"}
    return
  }
  
  access_token = jwt.sign({
    user_id: 1,
    exp: now() + 3600
  }, jwt_secret)
  
  refresh_token = jwt.sign({
    user_id: 1,
    type: "refresh",
    exp: now() + 604800
  }, jwt_secret)
  
  response.body = {
    access_token: access_token,
    token_type: "Bearer",
    expires_in: 3600,
    refresh_token: refresh_token
  }
}
```
