# Sessions

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
  user_id = session.user_id
  username = session.username
  
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
    session.user_id = 1
    session.username = username
    session.role = "admin"
    
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
  session.destroy()
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
    session.user_id = 1
    session.email = email
    session.logged_in_at = date()
    
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
  if !session.user_id {
    response.status = 401
    response.body = {error: "Not authenticated"}
    return
  }
  
  response.body = {
    user_id: session.user_id,
    email: session.email,
    logged_in_at: session.logged_in_at
  }
}

route POST "/auth/logout" {
  session.destroy()
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

fn require_auth() {
  if !session.user_id {
    response.status = 401
    response.body = {error: "Authentication required"}
    return false
  }
  return true
}

route GET "/public" {
  response.body = {message: "Public endpoint"}
}

route GET "/protected" {
  if !require_auth() {
    return
  }
  
  response.body = {
    message: "Protected data",
    user: session.username
  }
}

route DELETE "/account" {
  if !require_auth() {
    return
  }
  
  user_id = session.user_id
  session.destroy()
  
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
    session.user_id = 1
    session.username = username
    session.created_at = date("unix")
    
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}

route GET "/dashboard" {
  if !session.user_id {
    response.status = 401
    response.body = {error: "Not authenticated"}
    return
  }
  
  created_at = session.created_at ?? 0
  age = date("unix") - created_at
  
  response.body = {
    user: session.username,
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

db_conn = db.open("sqlite", "./sessions.db")

db_conn.exec(`
  CREATE TABLE IF NOT EXISTS sessions (
    key TEXT PRIMARY KEY,
    value TEXT,
    expires_at INTEGER
  )
`, [])

set_session_store(db_conn, "sessions", 60)

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

route GET "/profile" {
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
  cart = session.cart ?? []
  
  response.body = {
    items: cart,
    count: len(cart)
  }
}

route POST "/cart/add" json {
  cart = session.cart ?? []
  
  item = request.data
  cart = append(cart, item)
  
  session.cart = cart
  
  response.body = {
    items: cart,
    count: len(cart)
  }
}

route DELETE "/cart/clear" {
  session.cart = []
  response.body = {message: "Cart cleared"}
}

route DELETE "/cart/item/:index" {
  cart = session.cart ?? []
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
  
  session.cart = new_cart
  
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
    theme: session.theme ?? "light",
    language: session.language ?? "en",
    notifications: session.notifications ?? true
  }
  
  response.body = prefs
}

route PUT "/preferences" json {
  {theme, language, notifications} = request.data
  
  if theme != null {
    session.theme = theme
  }
  
  if language != null {
    session.language = language
  }
  
  if notifications != null {
    session.notifications = notifications
  }
  
  response.body = {
    theme: session.theme,
    language: session.language,
    notifications: session.notifications
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
  session.flash_message = "Form submitted successfully!"
  session.flash_type = "success"
  
  redirect("/form")
}

route GET "/form" {
  flash = session.flash_message ?? ""
  flash_type = session.flash_type ?? ""
  
  session.flash_message = null
  session.flash_type = null
  
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
    session.user_id = 1
    session.username = username
    session.login_time = date()
    
    response.body = {success: true}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```
