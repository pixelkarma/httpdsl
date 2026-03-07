# Templates

HTTPDSL uses Go's `html/template` engine for rendering HTML templates.

## Configuration

Specify the templates directory in server config:

```httpdsl
server {
  port 3000
  templates "./templates"
}
```

## Basic Rendering

Use `render()` to render templates:

```httpdsl
server {
  port 3000
  templates "./templates"
}

route GET "/" {
  render("index.gohtml", {
    title: "Home Page",
    message: "Welcome to HTTPDSL"
  })
}
```

Template file `templates/index.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>{{.Page.title}}</title>
  </head>
  <body>
    <h1>{{.Page.message}}</h1>
  </body>
</html>
```

## Template Data

The second argument to `render()` is available as `.Page`:

```httpdsl
route GET "/user/:id" {
  user_id = int(request.params.id)
  
  render("user.gohtml", {
    id: user_id,
    name: "Alice",
    email: "alice@example.com",
    active: true
  })
}
```

Template:

```html
<div class="user">
  <h1>{{.Page.name}}</h1>
  <p>Email: {{.Page.email}}</p>
  <p>ID: {{.Page.id}}</p>
  {{if .Page.active}}
    <span class="badge">Active</span>
  {{end}}
</div>
```

## Request Data

Access request information via `.Request`:

```httpdsl
route GET "/debug" {
  render("debug.gohtml", {})
}
```

Template:

```html
<dl>
  <dt>Method</dt>
  <dd>{{.Request.method}}</dd>
  
  <dt>Path</dt>
  <dd>{{.Request.path}}</dd>
  
  <dt>IP Address</dt>
  <dd>{{.Request.ip}}</dd>
</dl>
```

## Conditional Rendering

```httpdsl
route GET "/dashboard" {
  is_admin = request.session.role == "admin"
  
  render("dashboard.gohtml", {
    username: request.session.username ?? "Guest",
    is_admin: is_admin,
    notifications: 5
  })
}
```

Template:

```html
<h1>Dashboard</h1>
<p>Welcome, {{.Page.username}}!</p>

{{if .Page.is_admin}}
  <a href="/admin">Admin Panel</a>
{{end}}

{{if gt .Page.notifications 0}}
  <div class="badge">{{.Page.notifications}} new notifications</div>
{{end}}
```

## Loops in Templates

```httpdsl
route GET "/users" {
  users = [
    {id: 1, name: "Alice"},
    {id: 2, name: "Bob"},
    {id: 3, name: "Charlie"}
  ]
  
  render("users.gohtml", {users: users})
}
```

Template:

```html
<h1>Users</h1>
<ul>
  {{range .Page.users}}
    <li>{{.id}}: {{.name}}</li>
  {{end}}
</ul>
```

With empty check:

```html
{{if .Page.users}}
  <ul>
    {{range .Page.users}}
      <li>{{.name}}</li>
    {{end}}
  </ul>
{{else}}
  <p>No users found.</p>
{{end}}
```

## CSRF Protection

For forms, use CSRF helpers:

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
  render("form.gohtml", {action: "/submit"})
}

route POST "/submit" form {
  response.body = {received: request.data}
}
```

Template with CSRF field:

```html
<form method="POST" action="{{.Page.action}}">
  {{csrf_field}}
  
  <input type="text" name="username" placeholder="Username">
  <input type="password" name="password" placeholder="Password">
  <button type="submit">Submit</button>
</form>
```

Or manual token:

```html
<form method="POST" action="/submit">
  <input type="hidden" name="_csrf" value="{{csrf_token}}">
  <input type="text" name="message">
  <button type="submit">Send</button>
</form>
```

## Render as Expression

Use `render()` as an expression to get rendered HTML as string:

```httpdsl
route GET "/email-preview" {
  html = render("email.gohtml", {
    name: "Alice",
    subject: "Welcome!"
  })
  
  response.body = {html: html}
}
```

## Layout Templates

Create a base layout `templates/layout.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>{{.Page.title}}</title>
    <link rel="stylesheet" href="/assets/style.css">
  </head>
  <body>
    <nav>
      <a href="/">Home</a>
      <a href="/about">About</a>
    </nav>
    
    <main>
      {{template "content" .}}
    </main>
    
    <footer>
      <p>&copy; 2024 My Site</p>
    </footer>
  </body>
</html>
```

Content template `templates/home.gohtml`:

```html
{{define "content"}}
  <h1>{{.Page.heading}}</h1>
  <p>{{.Page.message}}</p>
{{end}}
```

Render:

```httpdsl
route GET "/" {
  render("layout.gohtml", {
    title: "Home",
    heading: "Welcome",
    message: "This is the home page"
  })
}
```

## Template Functions

Go template functions available:

```html
<!-- Comparison -->
{{if eq .Page.status "active"}}Active{{end}}
{{if ne .Page.count 0}}Count: {{.Page.count}}{{end}}
{{if gt .Page.score 100}}High score!{{end}}
{{if lt .Page.age 18}}Minor{{end}}

<!-- Logic -->
{{if and .Page.logged_in .Page.verified}}Welcome!{{end}}
{{if or .Page.is_admin .Page.is_moderator}}Admin area{{end}}
{{if not .Page.disabled}}Enabled{{end}}

<!-- String functions -->
{{.Page.name | printf "%s!"}}
{{.Page.text | print}}
```

### safe / safeJS

By default, Go's `html/template` escapes values based on context. Use `safe` and `safeJS` to bypass escaping when you trust the content:

```html
<!-- Render raw HTML (no escaping) -->
{{.Page.html_content | safe}}

<!-- Embed values in JavaScript -->
<script>
  var config = {
    name: "{{.Page.name | safeJS}}",
    count: {{.Page.count | safeJS}}
  };
</script>
```

- `safe` returns `template.HTML` — use for trusted HTML content
- `safeJS` returns `template.JS` — use for embedding values in `<script>` tags

> **Warning:** Only use these with trusted data. Never pipe user input through `safe` or `safeJS`.

## Complete Example

```httpdsl
server {
  port 3000
  templates "./templates"
  static "/assets" "./public"
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")
    csrf true
  }
}

route GET "/" {
  render("home.gohtml", {
    title: "Home",
    logged_in: request.session.user_id != null,
    username: request.session.username ?? "Guest"
  })
}

route GET "/posts" {
  posts = [
    {id: 1, title: "First Post", author: "Alice"},
    {id: 2, title: "Second Post", author: "Bob"}
  ]
  
  render("posts.gohtml", {
    title: "Blog Posts",
    posts: posts,
    count: len(posts)
  })
}

route GET "/posts/:id" {
  post_id = int(request.params.id)
  
  post = {
    id: post_id,
    title: "Post " + str(post_id),
    content: "This is the content of post " + str(post_id),
    author: "Alice",
    created_at: now()
  }
  
  render("post.gohtml", {
    title: post.title,
    post: post
  })
}

route GET "/login" {
  render("login.gohtml", {title: "Login"})
}

route POST "/login" form {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    request.session.user_id = 1
    request.session.username = username
    redirect("/dashboard")
  } else {
    render("login.gohtml", {
      title: "Login",
      error: "Invalid credentials"
    })
  }
}

route GET "/dashboard" {
  if !request.session.user_id {
    redirect("/login")
  }
  
  render("dashboard.gohtml", {
    title: "Dashboard",
    username: request.session.username
  })
}
```

Template `templates/home.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>{{.Page.title}}</title>
  </head>
  <body>
    <h1>Welcome{{if .Page.logged_in}}, {{.Page.username}}{{end}}!</h1>
    
    {{if .Page.logged_in}}
      <a href="/dashboard">Dashboard</a>
      <a href="/logout">Logout</a>
    {{else}}
      <a href="/login">Login</a>
    {{end}}
    
    <p><a href="/posts">View Posts</a></p>
  </body>
</html>
```

Template `templates/posts.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>{{.Page.title}}</title>
  </head>
  <body>
    <h1>Blog Posts</h1>
    <p>Total: {{.Page.count}}</p>
    
    {{if .Page.posts}}
      <ul>
        {{range .Page.posts}}
          <li>
            <a href="/posts/{{.id}}">{{.title}}</a>
            by {{.author}}
          </li>
        {{end}}
      </ul>
    {{else}}
      <p>No posts yet.</p>
    {{end}}
  </body>
</html>
```

Template `templates/login.gohtml`:

```html
<!DOCTYPE html>
<html>
  <head>
    <title>{{.Page.title}}</title>
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
