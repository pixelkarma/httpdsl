# Routes

Routes define HTTP endpoints in your application. Each route specifies a method, path, and handler.

## Basic Routes

```httpdsl
server {
  port 3000
}

route GET "/" {
  response.body = "Home page"
}

route GET "/about" {
  response.body = "About page"
}
```

## HTTP Methods

Supported methods:

```httpdsl
route GET "/items" {
  response.body = {items: [1, 2, 3]}
}

route POST "/items" {
  response.body = {created: true}
}

route PUT "/items/:id" {
  response.body = {updated: true}
}

route PATCH "/items/:id" {
  response.body = {patched: true}
}

route DELETE "/items/:id" {
  response.body = {deleted: true}
}
```

## Path Parameters

Capture dynamic segments with `:name`:

```httpdsl
route GET "/users/:id" {
  user_id = request.params.id
  response.body = {user_id: user_id}
}

route GET "/posts/:post_id/comments/:comment_id" {
  post_id = request.params.post_id
  comment_id = request.params.comment_id
  
  response.body = {
    post_id: post_id,
    comment_id: comment_id
  }
}
```

## Wildcard Routes

Capture the rest of the path with `*name`:

```httpdsl
route GET "/files/*path" {
  file_path = request.params.path
  response.body = {path: file_path}
}
```

Examples:
- `/files/docs/readme.md` → `path = "docs/readme.md"`
- `/files/images/logo.png` → `path = "images/logo.png"`

## Content Type Hints

Specify expected request body format:

### JSON

```httpdsl
route POST "/api/users" json {
  name = request.data.name
  email = request.data.email
  
  response.body = {
    id: cuid2(),
    name: name,
    email: email
  }
}
```

### Text

```httpdsl
route POST "/api/echo" text {
  content = request.data
  response.body = content
}
```

### Form

```httpdsl
route POST "/submit" form {
  username = request.data.username
  password = request.data.password
  
  response.body = {received: true}
}
```

## Route Timeouts

Set a timeout for a specific route:

```httpdsl
route GET "/slow" {
  timeout 5
  
  sleep(3000)
  response.body = "Completed"
}
```

Timeout is specified in seconds. If exceeded, the request is aborted.

## CSRF Override

Disable CSRF protection for specific routes:

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret "secret-key"
    csrf true
  }
}

route POST "/api/webhook" json {
  csrf false
  
  response.body = {received: true}
}
```

## Complete Route Example

```httpdsl
route POST "/api/posts/:id/comments" json {
  timeout 10
  
  post_id = int(request.params.id)
  {author, content} = request.data
  
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
    created_at: now()
  }
  
  response.status = 201
  response.body = comment
}
```

## Query Parameters

Access via `request.query`:

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

## Multiple Routes Same Path

Different methods can share the same path:

```httpdsl
route GET "/api/items/:id" {
  item_id = request.params.id
  response.body = {id: item_id, name: "Item " + item_id}
}

route PUT "/api/items/:id" json {
  item_id = request.params.id
  name = request.data.name
  
  response.body = {id: item_id, name: name, updated: true}
}

route DELETE "/api/items/:id" {
  item_id = request.params.id
  response.body = {id: item_id, deleted: true}
}
```

## REST API Example

```httpdsl
server {
  port 3000
}

items = []

route GET "/api/items" {
  response.body = {items: items, count: len(items)}
}

route POST "/api/items" json {
  name = request.data.name ?? ""
  
  if name == "" {
    response.status = 400
    response.body = {error: "Name is required"}
    return
  }
  
  item = {
    id: cuid2(),
    name: name,
    created_at: now()
  }
  
  items = append(items, item)
  
  response.status = 201
  response.body = item
}

route GET "/api/items/:id" {
  item_id = request.params.id
  found = null
  
  each item in items {
    if item.id == item_id {
      found = item
      break
    }
  }
  
  if found == null {
    response.status = 404
    response.body = {error: "Item not found"}
  } else {
    response.body = found
  }
}

route PUT "/api/items/:id" json {
  item_id = request.params.id
  new_name = request.data.name ?? ""
  
  if new_name == "" {
    response.status = 400
    response.body = {error: "Name is required"}
    return
  }
  
  updated = false
  i = 0
  
  each item in items {
    if item.id == item_id {
      items[i].name = new_name
      items[i].updated_at = now()
      updated = true
      break
    }
    i += 1
  }
  
  if !updated {
    response.status = 404
    response.body = {error: "Item not found"}
  } else {
    response.body = items[i]
  }
}

route DELETE "/api/items/:id" {
  item_id = request.params.id
  new_items = []
  deleted = false
  
  each item in items {
    if item.id == item_id {
      deleted = true
    } else {
      new_items = append(new_items, item)
    }
  }
  
  if !deleted {
    response.status = 404
    response.body = {error: "Item not found"}
  } else {
    items = new_items
    response.body = {deleted: true}
  }
}
```

## SSE Routes

Server-Sent Events for real-time streaming:

```httpdsl
route SSE "/events" {
  stream.send("welcome", {message: "Connected"})
}
```

See [SSE documentation](sse.md) for details.
