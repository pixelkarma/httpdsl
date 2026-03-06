# CSRF Protection

Cross-Site Request Forgery (CSRF) protection prevents unauthorized actions from malicious sites.

## Enable CSRF

Enable CSRF in session configuration:

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
    csrf true
  }
}
```

## Automatic Validation

CSRF tokens are automatically validated for:
- POST
- PUT
- DELETE
- PATCH

GET, HEAD, OPTIONS requests are not validated.

## Token Sources

CSRF tokens are checked in this order:

1. `X-CSRF-Token` header
2. `X-XSRF-Token` header
3. `_csrf` query parameter
4. `_csrf` form field
5. `_csrf` JSON body field

## Form Usage

Use `csrf_field` in templates:

```httpdsl
server {
  port 3000
  templates "./templates"
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
    csrf true
  }
}

route GET "/form" {
  render("form.gohtml", {})
}

route POST "/submit" form {
  name = request.data.name
  response.body = {received: name}
}
```

Template `templates/form.gohtml`:

```html
<form method="POST" action="/submit">
  {{csrf_field}}
  
  <input type="text" name="name" placeholder="Your name">
  <button type="submit">Submit</button>
</form>
```

The `csrf_field` helper generates:

```html
<input type="hidden" name="_csrf" value="token-value-here">
```

## Manual Token

Use `csrf_token()` to get the token value:

```httpdsl
route GET "/api/token" {
  token = csrf_token()
  response.body = {csrf_token: token}
}
```

In templates:

```html
<form method="POST" action="/submit">
  <input type="hidden" name="_csrf" value="{{csrf_token}}">
  <input type="text" name="message">
  <button type="submit">Send</button>
</form>
```

## AJAX Requests

Include CSRF token in headers:

```javascript
fetch('/api/data', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-CSRF-Token': document.querySelector('[name="_csrf"]').value
  },
  body: JSON.stringify({data: 'value'})
})
```

Or embed in page:

```httpdsl
route GET "/app" {
  render("app.gohtml", {csrf: csrf_token()})
}
```

Template:

```html
<script>
  const csrfToken = '{{.Page.csrf}}';
  
  fetch('/api/data', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': csrfToken
    },
    body: JSON.stringify({data: 'value'})
  });
</script>
```

## Disable for Specific Routes

Disable CSRF for API endpoints:

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
    csrf true
  }
}

route POST "/api/webhook" json {
  csrf false
  
  data = request.data
  response.body = {received: true}
}

route POST "/api/public" json {
  csrf false
  
  response.body = {message: "No CSRF required"}
}
```

## Safe Origins

Trust specific origins to bypass CSRF:

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
    csrf true
    csrf_safe_origins [
      "https://trusted-app.example.com",
      "https://mobile.example.com"
    ]
  }
}

route POST "/api/data" json {
  response.body = {received: request.data}
}
```

Requests from safe origins skip CSRF validation.

## Complete Example

```httpdsl
server {
  port 3000
  templates "./templates"
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")
    csrf true
  }
}

route GET "/" {
  render("home.gohtml", {
    logged_in: request.session.user_id != null,
    username: request.session.username ?? "Guest"
  })
}

route GET "/login" {
  render("login.gohtml", {})
}

route POST "/login" form {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    request.session.user_id = 1
    request.session.username = username
    redirect("/dashboard")
  } else {
    render("login.gohtml", {error: "Invalid credentials"})
  }
}

route GET "/dashboard" {
  if !request.session.user_id {
    redirect("/login")
  }
  
  render("dashboard.gohtml", {username: request.session.username})
}

route POST "/logout" form {
  request.session.destroy()
  redirect("/")
}

route GET "/settings" {
  if !request.session.user_id {
    redirect("/login")
  }
  
  render("settings.gohtml", {
    email: request.session.email ?? "",
    notifications: request.session.notifications ?? true
  })
}

route POST "/settings" form {
  if !request.session.user_id {
    redirect("/login")
  }
  
  request.session.email = request.data.email
  request.session.notifications = request.data.notifications == "on"
  
  redirect("/settings")
}
```

Template `templates/login.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>Login</title>
  </head>
  <body>
    <h1>Login</h1>
    
    {{if .Page.error}}
      <div class="error">{{.Page.error}}</div>
    {{end}}
    
    <form method="POST" action="/login">
      {{csrf_field}}
      
      <div>
        <label>Username:</label>
        <input type="text" name="username" required>
      </div>
      
      <div>
        <label>Password:</label>
        <input type="password" name="password" required>
      </div>
      
      <button type="submit">Login</button>
    </form>
  </body>
</html>
```

Template `templates/dashboard.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>Dashboard</title>
  </head>
  <body>
    <h1>Welcome, {{.Page.username}}!</h1>
    
    <nav>
      <a href="/settings">Settings</a>
    </nav>
    
    <form method="POST" action="/logout">
      {{csrf_field}}
      <button type="submit">Logout</button>
    </form>
  </body>
</html>
```

Template `templates/settings.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>Settings</title>
  </head>
  <body>
    <h1>Settings</h1>
    
    <form method="POST" action="/settings">
      {{csrf_field}}
      
      <div>
        <label>Email:</label>
        <input type="email" name="email" value="{{.Page.email}}">
      </div>
      
      <div>
        <label>
          <input type="checkbox" name="notifications" {{if .Page.notifications}}checked{{end}}>
          Enable notifications
        </label>
      </div>
      
      <button type="submit">Save</button>
    </form>
  </body>
</html>
```

## SPA with CSRF

For Single Page Applications:

```httpdsl
server {
  port 3000
  templates "./templates"
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
    csrf true
  }
}

route GET "/app" {
  render("app.gohtml", {
    csrf_token: csrf_token()
  })
}

route POST "/api/data" json {
  response.body = {
    received: request.data,
    timestamp: now()
  }
}
```

Template `templates/app.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>App</title>
  </head>
  <body>
    <div id="app"></div>
    
    <script>
      const CSRF_TOKEN = '{{.Page.csrf_token}}';
      
      async function apiPost(endpoint, data) {
        const response = await fetch(endpoint, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': CSRF_TOKEN
          },
          body: JSON.stringify(data)
        });
        
        return response.json();
      }
      
      // Usage
      apiPost('/api/data', {message: 'Hello'})
        .then(data => console.log(data));
    </script>
  </body>
</html>
```

## Testing CSRF

Test CSRF protection:

```bash
# Without token - should fail
curl -X POST http://localhost:3000/submit \
  -d "name=test"

# With valid token - should succeed
curl -X POST http://localhost:3000/submit \
  -H "X-CSRF-Token: valid-token-here" \
  -d "name=test"
```
