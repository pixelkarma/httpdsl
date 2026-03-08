# Store

- [Basic Operations](#basic-operations)
  - [set()](#set)
  - [get()](#get)
  - [has()](#has)
  - [delete()](#delete)
  - [incr()](#incr)
  - [Getting All Keys](#getting-all-keys)
- [Persistence](#persistence)
  - [File Sync](#file-sync)
  - [Database Sync](#database-sync)
- [Session Storage](#session-storage)
- [Caching Example](#caching-example)
- [Rate Limiting](#rate-limiting)
- [Counter Example](#counter-example)
- [Feature Flags](#feature-flags)
- [Analytics](#analytics)
- [Temporary Data](#temporary-data)
- [Configuration Storage](#configuration-storage)

The in-memory store provides key-value storage with optional TTL and persistence.

## Basic Operations

### set()

Store a value:

```httpdsl
store.set("key", "value")
store.set("counter", 0)
store.set("user", {id: 1, name: "Alice"})
```

With TTL (time-to-live in seconds):

```httpdsl
store.set("session", "token123", 3600)
store.set("cache", data, 300)
```

### get()

Retrieve a value:

```httpdsl
value = store.get("key")

if value != null {
  log_info(value)
}
```

With default value:

```httpdsl
count = store.get("counter", 0)
name = store.get("username", "Guest")
```

### has()

Check if key exists:

```httpdsl
if store.has("key") {
  log_info("Key exists")
}
```

### delete()

Remove a key:

```httpdsl
store.delete("key")
```

### incr()

Increment a numeric value:

```httpdsl
store.set("counter", 0)
store.incr("counter", 1)
store.incr("counter", 5)

count = store.get("counter")
```

With TTL:

```httpdsl
store.incr("rate_limit", 1, 60)
```

### Getting All Keys

Use `keys()` on `store.all()` to list all non-expired keys:

```httpdsl
all_keys = keys(store.all())

each key in all_keys {
  log_info(key)
}
```

## Persistence

### File Sync

Persist to JSON file:

```httpdsl
store.sync("./store.json")
```

With auto-flush interval (seconds):

```httpdsl
store.sync("./store.json", 60)
```

### Database Sync

Persist to database (table is auto-created):

```httpdsl
db_conn = db.open("sqlite", "./app.db")
store.sync(db_conn, "store")
```

With flush interval:

```httpdsl
store.sync(db_conn, "store", 30)
```

## Session Storage

Persist sessions to store:

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")
  }
}

db_conn = db.open("sqlite", "./app.db")
set_session_store(db_conn, "sessions", 60)

route GET "/" {
  visits = store.get("visits", 0)
  store.incr("visits", 1)
  
  response.body = {visits: visits + 1}
}
```

## Caching Example

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

fn get_user(id) {
  cache_key = `user:${id}`
  
  cached = store.get(cache_key)
  
  if cached != null {
    log_info("Cache hit")
    return cached
  }
  
  log_info("Cache miss")
  
  user = db_conn.query_one(
    "SELECT * FROM users WHERE id = ?",
    [id]
  )
  
  if user != null {
    store.set(cache_key, user, 300)
  }
  
  return user
}

route GET "/users/:id" {
  user_id = int(request.params.id)
  user = get_user(user_id)
  
  if user == null {
    response.status = 404
    response.body = {error: "User not found"}
  } else {
    response.body = user
  }
}

route PUT "/users/:id" json {
  user_id = int(request.params.id)
  {name, email} = request.data
  
  result = db_conn.exec(
    "UPDATE users SET name = ?, email = ? WHERE id = ?",
    [name, email, user_id]
  )
  
  cache_key = `user:${user_id}`
  store.delete(cache_key)
  
  response.body = {updated: true}
}
```

## Rate Limiting

```httpdsl
server {
  port 3000
}

before {
  client_ip = request.ip
  rate_key = `rate:${client_ip}`
  
  count = store.get(rate_key, 0)
  
  if count >= 100 {
    response.status = 429
    response.body = {error: "Too many requests"}
    return
  }
  
  store.incr(rate_key, 1, 60)
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Counter Example

```httpdsl
server {
  port 3000
}

store.sync("./counters.json", 10)

route GET "/counter/:name" {
  name = request.params.name
  count = store.get(name, 0)
  
  response.body = {name: name, count: count}
}

route POST "/counter/:name/increment" {
  name = request.params.name
  
  store.incr(name, 1)
  count = store.get(name)
  
  response.body = {name: name, count: count}
}

route POST "/counter/:name/decrement" {
  name = request.params.name
  
  store.incr(name, -1)
  count = store.get(name)
  
  response.body = {name: name, count: count}
}

route DELETE "/counter/:name" {
  name = request.params.name
  store.delete(name)
  
  response.body = {deleted: name}
}
```

## Feature Flags

```httpdsl
server {
  port 3000
}

store.set("feature_new_ui", true)
store.set("feature_beta", false)

route GET "/api/features" {
  all_keys = keys(store.all())
  features = {}
  
  each key in all_keys {
    if starts_with(key, "feature_") {
      features[key] = store.get(key)
    }
  }
  
  response.body = features
}

route PUT "/admin/feature/:name" json {
  feature_name = `feature_${request.params.name}`
  enabled = request.data.enabled
  
  store.set(feature_name, enabled)
  
  response.body = {feature: feature_name, enabled: enabled}
}

route GET "/" {
  new_ui = store.get("feature_new_ui", false)
  
  if new_ui {
    response.body = "New UI"
  } else {
    response.body = "Old UI"
  }
}
```

## Analytics

```httpdsl
server {
  port 3000
}

store.sync("./analytics.json", 60)

before {
  path = request.path
  store.incr(`views:${path}`, 1)
  store.incr("total_requests", 1)
}

route GET "/" {
  response.body = "Home"
}

route GET "/about" {
  response.body = "About"
}

route GET "/admin/stats" {
  all_keys = keys(store.all())
  views = {}
  
  each key in all_keys {
    if starts_with(key, "views:") {
      path = slice(key, 6, len(key))
      views[path] = store.get(key)
    }
  }
  
  response.body = {
    total_requests: store.get("total_requests", 0),
    views: views
  }
}
```

## Temporary Data

```httpdsl
server {
  port 3000
}

route POST "/share" json {
  content = request.data.content
  share_id = cuid2()
  
  store.set(`share:${share_id}`, content, 3600)
  
  response.body = {
    share_id: share_id,
    expires_in: 3600
  }
}

route GET "/share/:id" {
  share_id = request.params.id
  content = store.get(`share:${share_id}`)
  
  if content == null {
    response.status = 404
    response.body = {error: "Share not found or expired"}
  } else {
    response.body = {content: content}
  }
}
```

## Configuration Storage

```httpdsl
server {
  port 3000
}

store.sync("./config.json")

store.set("site_name", "My Site")
store.set("maintenance_mode", false)
store.set("max_upload_size", 10485760)

before {
  if store.get("maintenance_mode", false) {
    response.status = 503
    response.body = {error: "Site under maintenance"}
    return
  }
}

route GET "/config" {
  response.body = {
    site_name: store.get("site_name"),
    maintenance_mode: store.get("maintenance_mode"),
    max_upload_size: store.get("max_upload_size")
  }
}

route PUT "/admin/config" json {
  {site_name, maintenance_mode, max_upload_size} = request.data
  
  if site_name != null {
    store.set("site_name", site_name)
  }
  
  if maintenance_mode != null {
    store.set("maintenance_mode", maintenance_mode)
  }
  
  if max_upload_size != null {
    store.set("max_upload_size", max_upload_size)
  }
  
  response.body = {success: true}
}
```
