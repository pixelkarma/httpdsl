# Sessions

- [Configuration](#configuration)
- [Session Options](#session-options)
- [Reading Session Data](#reading-session-data)
- [Writing Session Data](#writing-session-data)
- [Destroying Sessions](#destroying-sessions)
- [Authentication Example](#authentication-example)
- [Protected Routes](#protected-routes)
- [Session Expiration](#session-expiration)
- [Session with Database Persistence](#session-with-database-persistence)
- [Shopping Cart Example](#shopping-cart-example)
- [User Preferences](#user-preferences)
- [Flash Messages](#flash-messages)
- [Multiple Sessions](#multiple-sessions)

HTTPDSL provides built-in session management with cookie-based storage.

## Configuration

Enable sessions in the server block:

```httpdsl
server {
  port 3000
  session {
    cookie "session_id"
    expires 24 h
    secret env("SESSION_SECRET")
  }
}
```

## Session Options

- `cookie`: Cookie name (default: `"sid"`)
- `expires`: Session duration (e.g., `1 h`, `30 m`, `7 d`)
- `secret`: Secret key for signing sessions (required for production)
- `csrf`: Enable CSRF protection (default: `false`)
- `csrf_safe_origins`: Array of trusted origins that bypass CSRF checks

## Reading Session Data

Access session values:

```httpdsl
route GET "/profile" {
  user_id = request.session.user_id
  username = request.session.username
  
  if user_id == null {
    response.status = 401
    response.body = {error: "Not logged in"}
  } else {
    response.body = {
      user_id: user_id,
      username: username
    }
  }
}
```

## Writing Session Data

Set session values:

```httpdsl
route POST "/login" json {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    request.session.user_id = 1
    request.session.username = username
    request.session.role = "admin"
    
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```

## Destroying Sessions

Clear all session data:

```httpdsl
route POST "/logout" {
  request.session.destroy()
  response.body = {message: "Logged out successfully"}
}
```

## Authentication Example

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET", "dev-secret-key")
  }
}

route POST "/auth/login" json {
  {email, password} = request.data
  
  if email == "" || password == "" {
    response.status = 400
    response.body = {error: "Email and password required"}
    return
  }
  
  if email == "user@example.com" && password == "password123" {
    request.session.user_id = 1
    request.session.email = email
    request.session.logged_in_at = now()
    
    response.body = {
      success: true,
      message: "Logged in successfully"
    }
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}

route GET "/auth/me" {
  if !request.session.user_id {
    response.status = 401
    response.body = {error: "Not authenticated"}
    return
  }
  
  response.body = {
    user_id: request.session.user_id,
    email: request.session.email,
    logged_in_at: request.session.logged_in_at
  }
}

route POST "/auth/logout" {
  request.session.destroy()
  response.body = {message: "Logged out"}
}
```

## Protected Routes

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
  is_public = request.path == "/public"
  if !is_public && !request.session.user_id {
    response.status = 401
    response.body = {error: "Authentication required"}
    return
  }
}

route GET "/public" {
  response.body = {message: "Public endpoint"}
}

route GET "/protected" {
  response.body = {
    message: "Protected data",
    user: request.session.username
  }
}

route DELETE "/account" {
  user_id = request.session.user_id
  request.session.destroy()
  
  response.body = {deleted: user_id}
}
```

## Session Expiration

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 30 m
    secret env("SESSION_SECRET")
  }
}

route POST "/login" json {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    request.session.user_id = 1
    request.session.username = username
    request.session.created_at = now()
    
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}

route GET "/dashboard" {
  if !request.session.user_id {
    response.status = 401
    response.body = {error: "Not authenticated"}
    return
  }
  
  created_at = request.session.created_at ?? 0
  age = now() - created_at
  
  response.body = {
    user: request.session.username,
    session_age: age
  }
}
```

## Session with Database Persistence

Store sessions in a database:

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 7 d
    secret env("SESSION_SECRET")
  }
}

init {
  db_conn = db.open("sqlite", "./sessions.db")

  db_conn.exec(`
    CREATE TABLE IF NOT EXISTS sessions (
      key TEXT PRIMARY KEY,
      value TEXT,
      expires_at INTEGER
    )
  `, [])

  set_session_store(db_conn, "sessions", 60)
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

route GET "/profile" {
  if !request.session.user_id {
    response.status = 401
    response.body = {error: "Not authenticated"}
    return
  }
  
  response.body = {
    user_id: request.session.user_id,
    username: request.session.username
  }
}
```

## Shopping Cart Example

```httpdsl
server {
  port 3000
  session {
    cookie "cart_session"
    expires 7 d
    secret env("SESSION_SECRET")
  }
}

route GET "/cart" {
  cart = request.session.cart ?? []
  
  response.body = {
    items: cart,
    count: len(cart)
  }
}

route POST "/cart/add" json {
  cart = request.session.cart ?? []
  
  item = request.data
  cart = append(cart, item)
  
  request.session.cart = cart
  
  response.body = {
    items: cart,
    count: len(cart)
  }
}

route DELETE "/cart/clear" {
  request.session.cart = []
  response.body = {message: "Cart cleared"}
}

route DELETE "/cart/item/:index" {
  cart = request.session.cart ?? []
  index = int(request.params.index)
  
  if index < 0 || index >= len(cart) {
    response.status = 404
    response.body = {error: "Item not found"}
    return
  }
  
  new_cart = []
  i = 0
  
  each item in cart {
    if i != index {
      new_cart = append(new_cart, item)
    }
    i += 1
  }
  
  request.session.cart = new_cart
  
  response.body = {
    items: new_cart,
    count: len(new_cart)
  }
}
```

## User Preferences

```httpdsl
server {
  port 3000
  session {
    cookie "prefs"
    expires 365 d
    secret env("SESSION_SECRET")
  }
}

route GET "/preferences" {
  prefs = {
    theme: request.session.theme ?? "light",
    language: request.session.language ?? "en",
    notifications: request.session.notifications ?? true
  }
  
  response.body = prefs
}

route PUT "/preferences" json {
  {theme, language, notifications} = request.data
  
  if theme != null {
    request.session.theme = theme
  }
  
  if language != null {
    request.session.language = language
  }
  
  if notifications != null {
    request.session.notifications = notifications
  }
  
  response.body = {
    theme: request.session.theme,
    language: request.session.language,
    notifications: request.session.notifications
  }
}
```

## Flash Messages

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
  }
}

route POST "/submit" form {
  request.session.flash_message = "Form submitted successfully!"
  request.session.flash_type = "success"
  
  redirect("/form")
}

route GET "/form" {
  flash = request.session.flash_message ?? ""
  flash_type = request.session.flash_type ?? ""
  
  request.session.flash_message = null
  request.session.flash_type = null
  
  response.body = {
    flash: flash,
    flash_type: flash_type
  }
}
```

## Multiple Sessions

Different session cookies for different purposes:

```httpdsl
server {
  port 3000
  session {
    cookie "auth_session"
    expires 1 h
    secret env("SESSION_SECRET")
  }
}

route POST "/login" json {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    request.session.user_id = 1
    request.session.username = username
    request.session.login_time = now()
    
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```
