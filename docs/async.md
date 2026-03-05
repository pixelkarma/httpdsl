# Async Operations

HTTPDSL provides async/await for concurrent operations.

## async Expression

Wrap expressions in `async` to execute asynchronously:

```httpdsl
f1 = async fetch("https://api.example.com/users")
f2 = async fetch("https://api.example.com/posts")

users = await(f1)
posts = await(f2)
```

## await Function

Wait for async operations to complete:

```httpdsl
future = async fetch("https://api.example.com/data")
result = await(future)

log_info(result.body)
```

## Multiple Awaits

Wait for multiple operations:

```httpdsl
f1 = async fetch("https://api.example.com/users")
f2 = async fetch("https://api.example.com/posts")
f3 = async fetch("https://api.example.com/comments")

users, posts, comments = await(f1, f2, f3)
```

## race Function

Return the first completed operation:

```httpdsl
f1 = async fetch("https://api1.example.com/data")
f2 = async fetch("https://api2.example.com/data")

winner = race(f1, f2)
log_info("First response received")
```

## Parallel Fetch Requests

```httpdsl
server {
  port 3000
}

route GET "/aggregate" {
  f1 = async fetch("https://api.example.com/users")
  f2 = async fetch("https://api.example.com/posts")
  f3 = async fetch("https://api.example.com/comments")
  
  users_res, posts_res, comments_res = await(f1, f2, f3)
  
  response.body = {
    users: users_res.body,
    posts: posts_res.body,
    comments: comments_res.body
  }
}
```

## Async Database Queries

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

route GET "/dashboard" {
  f1 = async db_conn.query("SELECT * FROM users", [])
  f2 = async db_conn.query("SELECT * FROM posts", [])
  f3 = async db_conn.query("SELECT * FROM comments", [])
  
  users, posts, comments = await(f1, f2, f3)
  
  response.body = {
    users_count: len(users),
    posts_count: len(posts),
    comments_count: len(comments)
  }
}
```

## Async File Operations

```httpdsl
server {
  port 3000
}

route GET "/files" {
  f1 = async file.read("./file1.txt")
  f2 = async file.read("./file2.txt")
  f3 = async file.read("./file3.txt")
  
  content1, content2, content3 = await(f1, f2, f3)
  
  response.body = {
    file1: content1,
    file2: content2,
    file3: content3
  }
}
```

## Async exec

```httpdsl
server {
  port 3000
}

route GET "/system-info" {
  f1 = async exec("uname -a")
  f2 = async exec("df -h")
  f3 = async exec("free -m")
  
  uname, disk, memory = await(f1, f2, f3)
  
  response.body = {
    system: uname.stdout,
    disk: disk.stdout,
    memory: memory.stdout
  }
}
```

## Race Condition Example

```httpdsl
server {
  port 3000
}

route GET "/fastest" {
  f1 = async fetch("https://api1.example.com/data")
  f2 = async fetch("https://api2.example.com/data")
  f3 = async fetch("https://api3.example.com/data")
  
  fastest = race(f1, f2, f3)
  
  response.body = fastest.body
}
```

## Timeout with Race

```httpdsl
server {
  port 3000
}

route GET "/with-timeout" {
  f1 = async fetch("https://slow-api.example.com/data")
  f2 = async sleep(5000)
  
  result = race(f1, f2)
  
  if type(result) == "null" {
    response.status = 504
    response.body = {error: "Request timed out"}
  } else {
    response.body = result.body
  }
}
```

## Error Handling with Async

```httpdsl
server {
  port 3000
}

route GET "/safe-fetch" {
  f1 = async fetch("https://api.example.com/users")
  f2 = async fetch("https://api.example.com/posts")
  
  try {
    users_res, posts_res = await(f1, f2)
    
    response.body = {
      users: users_res.body,
      posts: posts_res.body
    }
  } catch err {
    response.status = 500
    response.body = {error: "Failed to fetch data"}
  }
}
```

## Parallel Processing

```httpdsl
server {
  port 3000
}

fn process_item(item) {
  result = fetch(`https://api.example.com/process`, {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: item
  })
  return result.body
}

route POST "/batch" json {
  items = request.data.items
  
  futures = []
  
  each item in items {
    f = async process_item(item)
    futures = append(futures, f)
  }
  
  results = []
  
  each future in futures {
    result = await(future)
    results = append(results, result)
  }
  
  response.body = {
    processed: len(results),
    results: results
  }
}
```

## Map with Async

```httpdsl
server {
  port 3000
}

route GET "/users-with-posts" {
  users = [{id: 1}, {id: 2}, {id: 3}]
  
  futures = map(users, fn(user) {
    return async fetch(`https://api.example.com/users/${user.id}/posts`)
  })
  
  results = []
  
  each future in futures {
    result = await(future)
    results = append(results, result.body)
  }
  
  response.body = {results: results}
}
```

## Cache with Fallback

```httpdsl
server {
  port 3000
}

route GET "/data/:id" {
  data_id = request.params.id
  
  cached = store.get(`data:${data_id}`)
  
  if cached != null {
    response.body = cached
    return
  }
  
  f1 = async fetch(`https://primary-api.example.com/data/${data_id}`)
  f2 = async fetch(`https://backup-api.example.com/data/${data_id}`)
  
  result = race(f1, f2)
  
  if result.status == 200 {
    store.set(`data:${data_id}`, result.body, 300)
    response.body = result.body
  } else {
    response.status = 502
    response.body = {error: "Failed to fetch data"}
  }
}
```

## Webhook Fanout

```httpdsl
server {
  port 3000
}

webhook_urls = [
  "https://webhook1.example.com/notify",
  "https://webhook2.example.com/notify",
  "https://webhook3.example.com/notify"
]

route POST "/notify" json {
  payload = request.data
  
  futures = []
  
  each url in webhook_urls {
    f = async fetch(url, {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: payload
    })
    futures = append(futures, f)
  }
  
  results = []
  success_count = 0
  
  each future in futures {
    result = await(future)
    results = append(results, {status: result.status})
    
    if result.status >= 200 && result.status < 300 {
      success_count += 1
    }
  }
  
  response.body = {
    sent: len(webhook_urls),
    successful: success_count,
    results: results
  }
}
```

## Async Aggregation

```httpdsl
server {
  port 3000
}

route GET "/aggregate/:user_id" {
  user_id = request.params.user_id
  
  f1 = async fetch(`https://api.example.com/users/${user_id}`)
  f2 = async fetch(`https://api.example.com/users/${user_id}/posts`)
  f3 = async fetch(`https://api.example.com/users/${user_id}/comments`)
  f4 = async fetch(`https://api.example.com/users/${user_id}/likes`)
  
  user_res, posts_res, comments_res, likes_res = await(f1, f2, f3, f4)
  
  response.body = {
    user: user_res.body,
    posts: posts_res.body,
    comments: comments_res.body,
    likes: likes_res.body,
    total_activity: len(posts_res.body) + len(comments_res.body) + len(likes_res.body)
  }
}
```
