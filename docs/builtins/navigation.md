# Navigation Builtin

Function for HTTP redirects.

## redirect()

Redirect to another URL:

```httpdsl
redirect("/home")
redirect("/login")
redirect("https://example.com")
```

Default status: 302 (Found)

## With Custom Status

```httpdsl
redirect("/new-page", 301)
redirect("/temporary", 302)
redirect("/see-other", 303)
redirect("/moved", 307)
redirect("/permanent", 308)
```

Common status codes:
- `301` - Moved Permanently
- `302` - Found (temporary redirect)
- `303` - See Other
- `307` - Temporary Redirect
- `308` - Permanent Redirect

## Complete Examples

### Basic Redirect

```httpdsl
route GET "/old-page" {
  redirect("/new-page")
}
```

### Permanent Redirect

```httpdsl
route GET "/old-url" {
  redirect("/new-url", 301)
}
```

### Conditional Redirect

```httpdsl
route GET "/dashboard" {
  if !request.session.user_id {
    redirect("/login")
  }
  
  response.body = {message: "Dashboard"}
}
```

### After Login

```httpdsl
route POST "/auth/login" json {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    request.session.user_id = 1
    request.session.username = username
    
    redirect("/dashboard")
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```

### After Logout

```httpdsl
route POST "/auth/logout" {
  request.session.destroy()
  redirect("/")
}
```

### External Redirect

```httpdsl
route GET "/external" {
  redirect("https://example.com")
}
```

### URL Shortener

```httpdsl
route GET "/s/:code" {
  code = request.params.code
  
  url = store.get(`short:${code}`)
  
  if url == null {
    response.status = 404
    response.body = {error: "Short URL not found"}
    return
  }
  
  store.incr(`clicks:${code}`, 1)
  
  redirect(url)
}

route POST "/shorten" json {
  url = request.data.url ?? ""
  
  if !is_url(url) {
    response.status = 400
    response.body = {error: "Invalid URL"}
    return
  }
  
  code = cuid2(8)
  store.set(`short:${code}`, url)
  
  response.body = {
    original: url,
    short: `/s/${code}`
  }
}
```

### Language Selection

```httpdsl
route GET "/" {
  lang = request.query.lang ?? request.session.lang ?? "en"
  
  if lang == "es" {
    redirect("/es/")
  } else if lang == "fr" {
    redirect("/fr/")
  } else {
    response.body = "English version"
  }
}
```

### Mobile Redirect

```httpdsl
route GET "/" {
  user_agent = lower(request.headers["user-agent"] ?? "")
  
  is_mobile = contains(user_agent, "mobile") || contains(user_agent, "android")
  
  if is_mobile {
    redirect("/mobile")
  }
  
  response.body = "Desktop version"
}
```

### Authentication Guard

```httpdsl
before {
  is_public = request.path == "/login" || request.path == "/register"
  if !is_public && !request.session.user_id {
    redirect("/login")
  }
}

route GET "/profile" {
  response.body = {
    user_id: request.session.user_id,
    username: request.session.username
  }
}

route GET "/settings" {
  response.body = {settings: {}}
}
```

### Role-Based Redirect

```httpdsl
route GET "/admin" {
  if !request.session.user_id {
    redirect("/login")
  }
  
  if request.session.role != "admin" {
    redirect("/dashboard")
  }
  
  response.body = {message: "Admin panel"}
}
```

### Form Submission Redirect

```httpdsl
route POST "/contact" form {
  {name, email, message} = request.data
  
  if name == "" || email == "" || message == "" {
    response.status = 400
    response.body = {error: "All fields required"}
    return
  }
  
  request.session.flash_message = "Thank you for your message!"
  
  redirect("/contact/success")
}

route GET "/contact/success" {
  message = request.session.flash_message ?? ""
  request.session.flash_message = null
  
  response.body = {message: message}
}
```

### Canonical URL

```httpdsl
route GET "/product" {
  id = request.query.id ?? ""
  
  if id != "" {
    redirect(`/products/${id}`, 301)
  }
  
  response.body = {error: "Product ID required"}
}
```

### Maintenance Mode

```httpdsl
maintenance_mode = env("MAINTENANCE", "false") == "true"

before {
  if maintenance_mode && request.path != "/maintenance" {
    redirect("/maintenance")
  }
}

route GET "/maintenance" {
  response.body = "Site under maintenance"
}
```

### OAuth Callback

```httpdsl
route GET "/auth/callback" {
  code = request.query.code ?? ""
  
  if code == "" {
    redirect("/login")
  }
  
  result = fetch("https://oauth.example.com/token", {
    method: "POST",
    body: {code: code, client_id: env("CLIENT_ID")}
  })
  
  if result.status != 200 {
    redirect("/login")
  }
  
  request.session.access_token = result.body.access_token
  
  redirect("/dashboard")
}
```

### Trailing Slash

```httpdsl
route GET "/*path" {
  path = request.params.path
  
  if ends_with(path, "/") && path != "/" {
    new_path = slice(path, 0, len(path) - 1)
    redirect(new_path, 301)
  }
  
  response.body = {path: path}
}
```

### Post-Registration

```httpdsl
route POST "/auth/register" json {
  {username, email, password} = request.data
  
  errors = []
  
  if len(password) < 8 {
    errors = append(errors, "Password too short")
  }
  
  if !is_email(email) {
    errors = append(errors, "Invalid email")
  }
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
    return
  }
  
  request.session.user_id = 1
  request.session.username = username
  
  redirect("/welcome")
}
```

### Query Parameter Redirect

```httpdsl
route GET "/redirect" {
  url = request.query.url ?? "/"
  
  if !is_url(url) && !starts_with(url, "/") {
    response.status = 400
    response.body = {error: "Invalid redirect URL"}
    return
  }
  
  redirect(url)
}
```

## Notes

- `redirect()` stops route execution
- Sets `Location` header automatically
- Response body is ignored after redirect
- Use 301 for permanent redirects (SEO benefit)
- Use 302 for temporary redirects (default)
