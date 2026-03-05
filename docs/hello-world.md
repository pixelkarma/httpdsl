# Hello World

HTTPDSL is a domain-specific language for building HTTP servers. Here's the simplest possible server:

```httpdsl
server {
  port 3000
}

route GET "/" {
  response.body = "Hello, World!"
}
```

Save this as `server.httpdsl` and run it:

```bash
httpdsl run server.httpdsl
```

Visit `http://localhost:3000` to see your server in action.

## JSON API Example

Responding with JSON is automatic when you assign a hash or array to `response.body`:

```httpdsl
server {
  port 3000
}

route GET "/api/users" {
  response.body = [
    {id: 1, name: "Alice"},
    {id: 2, name: "Bob"}
  ]
}

route GET "/api/users/:id" {
  user_id = int(request.params.id)
  response.body = {
    id: user_id,
    name: "User " + str(user_id)
  }
}
```

## Path Parameters

Extract dynamic segments from URLs:

```httpdsl
server {
  port 3000
}

route GET "/greet/:name" {
  name = request.params.name
  response.body = `Hello, ${name}!`
}
```

## Query Parameters

Access query string parameters via `request.query`:

```httpdsl
server {
  port 3000
}

route GET "/search" {
  query = request.query.q ?? "(empty)"
  response.body = {search: query}
}
```

Visit `/search?q=httpdsl` to see the query parameter in action.

## POST with JSON Body

Handle JSON request bodies:

```httpdsl
server {
  port 3000
}

route POST "/api/users" json {
  name = request.data.name
  email = request.data.email
  
  response.status = 201
  response.body = {
    id: cuid2(),
    name: name,
    email: email
  }
}
```

Test with curl:

```bash
curl -X POST http://localhost:3000/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com"}'
```

## Next Steps

- Learn about [server configuration](server.md)
- Explore [routing](routes.md) in depth
- Add [middleware](middleware.md) for cross-cutting concerns
- Work with [databases](databases.md)
- Use [templates](templates.md) for HTML rendering
