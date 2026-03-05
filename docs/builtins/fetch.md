# Fetch Builtin

HTTP client for making external requests.

## Basic Usage

```httpdsl
fetch("https://api.example.com/data")
```

Returns:
```httpdsl
{
  status: 200,
  body: {...},
  headers: {...},
  cookies: {...}
}
```

## With Options

```httpdsl
fetch("https://api.example.com/data", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Authorization": "Bearer token"
  },
  body: {key: "value"}
})
```

### Options

- `method`: HTTP method (default: `"GET"`)
- `headers`: Request headers (hash)
- `body`: Request body (string, hash, or array)
- Hash/array bodies are automatically JSON-encoded

## Complete Examples

### GET Request

```httpdsl
route GET "/proxy" {
  result = fetch("https://api.example.com/users")
  
  if result.status != 200 {
    response.status = 502
    response.body = {error: "External API failed"}
    return
  }
  
  response.body = result.body
}
```

### POST Request

```httpdsl
route POST "/api/forward" json {
  data = request.data
  
  result = fetch("https://api.example.com/submit", {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: data
  })
  
  response.status = result.status
  response.body = result.body
}
```

### With Authentication

```httpdsl
route GET "/api/external-data" {
  api_key = env("EXTERNAL_API_KEY")
  
  result = fetch("https://api.example.com/data", {
    headers: {
      "Authorization": `Bearer ${api_key}`
    }
  })
  
  if result.status == 401 {
    response.status = 502
    response.body = {error: "External API authentication failed"}
    return
  }
  
  response.body = result.body
}
```

### Error Handling

```httpdsl
route GET "/api/safe-fetch" {
  try {
    result = fetch("https://api.example.com/data")
    
    if result.status >= 400 {
      response.status = 502
      response.body = {
        error: "External API error",
        status: result.status
      }
      return
    }
    
    response.body = result.body
  } catch err {
    response.status = 502
    response.body = {
      error: "Failed to connect to external API",
      details: err
    }
  }
}
```

### Query Parameters

```httpdsl
route GET "/search" {
  query = request.query.q ?? ""
  
  encoded = url_encode(query)
  url = `https://api.example.com/search?q=${encoded}`
  
  result = fetch(url)
  
  response.body = result.body
}
```

### Multiple Requests

```httpdsl
route GET "/aggregate" {
  result1 = fetch("https://api.example.com/users")
  result2 = fetch("https://api.example.com/posts")
  
  response.body = {
    users: result1.body,
    posts: result2.body
  }
}
```

### Parallel Requests

```httpdsl
route GET "/parallel" {
  f1 = async fetch("https://api.example.com/users")
  f2 = async fetch("https://api.example.com/posts")
  f3 = async fetch("https://api.example.com/comments")
  
  r1, r2, r3 = await(f1, f2, f3)
  
  response.body = {
    users: r1.body,
    posts: r2.body,
    comments: r3.body
  }
}
```

### Webhook Sending

```httpdsl
route POST "/api/events" json {
  event = request.data
  webhook_url = env("WEBHOOK_URL")
  
  result = fetch(webhook_url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Event-Type": event.type ?? "generic"
    },
    body: event
  })
  
  if result.status >= 200 && result.status < 300 {
    response.body = {sent: true}
  } else {
    response.status = 502
    response.body = {error: "Webhook delivery failed"}
  }
}
```

### Proxy with Headers

```httpdsl
route GET "/proxy/*path" {
  path = request.params.path
  upstream = env("UPSTREAM_API", "https://api.example.com")
  
  url = `${upstream}/${path}`
  
  result = fetch(url, {
    headers: {
      "User-Agent": "HTTPDSL-Proxy/1.0",
      "X-Forwarded-For": request.ip
    }
  })
  
  response.status = result.status
  response.headers = result.headers
  response.body = result.body
}
```

### OAuth Token Exchange

```httpdsl
route POST "/auth/exchange" json {
  code = request.data.code ?? ""
  
  result = fetch("https://oauth.example.com/token", {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded"
    },
    body: `grant_type=authorization_code&code=${code}&client_id=${env("CLIENT_ID")}&client_secret=${env("CLIENT_SECRET")}`
  })
  
  if result.status != 200 {
    response.status = 401
    response.body = {error: "Token exchange failed"}
    return
  }
  
  response.body = result.body
}
```

### API Rate Limit Check

```httpdsl
route GET "/api/data" {
  result = fetch("https://api.example.com/data", {
    headers: {
      "Authorization": `Bearer ${env("API_KEY")}`
    }
  })
  
  rate_limit = result.headers["x-ratelimit-limit"] ?? "unknown"
  rate_remaining = result.headers["x-ratelimit-remaining"] ?? "unknown"
  
  response.headers = {
    "X-RateLimit-Limit": rate_limit,
    "X-RateLimit-Remaining": rate_remaining
  }
  
  response.body = result.body
}
```

### Caching External API

```httpdsl
route GET "/api/cached/:id" {
  id = request.params.id
  cache_key = `external:${id}`
  
  cached = store.get(cache_key)
  
  if cached != null {
    response.headers = {"X-Cache": "HIT"}
    response.body = cached
    return
  }
  
  result = fetch(`https://api.example.com/items/${id}`)
  
  if result.status == 200 {
    store.set(cache_key, result.body, 300)
    response.headers = {"X-Cache": "MISS"}
    response.body = result.body
  } else {
    response.status = result.status
    response.body = {error: "Failed to fetch data"}
  }
}
```

### GraphQL Query

```httpdsl
route POST "/graphql-proxy" json {
  query = request.data.query ?? ""
  variables = request.data.variables ?? {}
  
  result = fetch("https://api.example.com/graphql", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${env("GRAPHQL_TOKEN")}`
    },
    body: {
      query: query,
      variables: variables
    }
  })
  
  response.body = result.body
}
```

### File Upload to External API

```httpdsl
route POST "/upload" json {
  file_content = request.data.content ?? ""
  filename = request.data.filename ?? "file.txt"
  
  encoded = base64_encode(file_content)
  
  result = fetch("https://api.example.com/upload", {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: {
      filename: filename,
      content: encoded
    }
  })
  
  response.status = result.status
  response.body = result.body
}
```
