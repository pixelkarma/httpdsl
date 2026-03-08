# Logging Builtins

- [print()](#print)
- [log()](#log)
- [log_info()](#log_info)
- [log_warn()](#log_warn)
- [log_error()](#log_error)
- [sleep()](#sleep)
- [server_stats()](#server_stats)
- [Complete Examples](#complete-examples)
  - [Request Logging](#request-logging)
  - [Error Logging](#error-logging)
  - [Debug Logging](#debug-logging)
  - [Performance Monitoring](#performance-monitoring)
  - [Periodic Health Check](#periodic-health-check)
  - [Rate Limit Warnings](#rate-limit-warnings)
  - [Async Operation Logging](#async-operation-logging)
  - [Slow Query Detection](#slow-query-detection)
  - [Authentication Logging](#authentication-logging)
  - [Structured Logging](#structured-logging)
  - [Debug Sleep](#debug-sleep)
  - [Memory Monitoring](#memory-monitoring)
  - [Request Timing](#request-timing)
  - [Shutdown Logging](#shutdown-logging)

Functions for logging and debugging.

## print()

Output to stdout:

```httpdsl
print("Hello")
print("Value:", value)
print("User", user.name, "logged in")
```

Accepts multiple arguments, space-separated.

## log()

Log to stderr with timestamp and context:

```httpdsl
log("Server started")
log("Processing request")
log("Error occurred:", error_message)
```

## log_info()

Info level logging:

```httpdsl
log_info("User logged in")
log_info("Request processed successfully")
log_info("Cache hit for key:", key)
```

## log_warn()

Warning level logging:

```httpdsl
log_warn("Rate limit approaching")
log_warn("Deprecated API used")
log_warn("High memory usage:", memory_mb, "MB")
```

## log_error()

Error level logging:

```httpdsl
log_error("Database connection failed")
log_error("Invalid input:", input_value)
log_error("Fatal error:", error_details)
```

## sleep()

Pause execution (milliseconds):

```httpdsl
sleep(1000)
sleep(500)
sleep(100)
```

## server_stats()

Get server statistics:

```httpdsl
stats = server_stats()
```

Returns:
```httpdsl
{
  mem_alloc: 12345678,
  mem_alloc_mb: 11.77,
  goroutines: 42,
  gc_count: 10,
  uptime: 3600,
  uptime_human: "1h0m0s"
}
```

## Complete Examples

### Request Logging

```httpdsl
server {
  port 3000
}

before {
  log_info(`${request.method} ${request.path} from ${request.ip}`)
}

after {
  log_info(`Response status: ${response.status}`)
}

route GET "/" {
  response.body = "Home"
}
```

### Error Logging

```httpdsl
route POST "/api/data" json {
  try {
    data = request.data
    
    log_info("Processing data")
    
    response.body = {success: true}
  } catch(err) {
    log_error("Failed to process data:", err)
    
    response.status = 500
    response.body = {error: "Processing failed"}
  }
}
```

### Debug Logging

```httpdsl
debug_mode = env("DEBUG", "false") == "true"

route GET "/api/users/:id" {
  user_id = request.params.id
  
  if debug_mode {
    log_info("Fetching user:", user_id)
  }
  
  user = {id: user_id, name: "User " + user_id}
  
  if debug_mode {
    log_info("User data:", json.stringify(user))
  }
  
  response.body = user
}
```

### Performance Monitoring

```httpdsl
route GET "/stats" {
  stats = server_stats()
  
  if stats.mem_alloc_mb > 500 {
    log_warn(`High memory usage: ${stats.mem_alloc_mb} MB`)
  }
  
  if stats.goroutines > 1000 {
    log_warn(`High goroutine count: ${stats.goroutines}`)
  }
  
  response.body = stats
}
```

### Periodic Health Check

```httpdsl
every 1 m {
  stats = server_stats()
  
  log_info(`Health check - Memory: ${stats.mem_alloc_mb} MB, Goroutines: ${stats.goroutines}, Uptime: ${stats.uptime_human}`)
  
  if stats.mem_alloc_mb > 1000 {
    log_error("Critical memory usage!")
  }
}
```

### Rate Limit Warnings

```httpdsl
route GET "/api/data" {
  client_ip = request.ip
  rate_key = `rate:${client_ip}`
  
  count = store.get(rate_key, 0)
  limit = 100
  
  if count >= limit {
    log_warn(`Rate limit exceeded for ${client_ip}`)
    
    response.status = 429
    response.body = {error: "Too many requests"}
    return
  }
  
  if count >= limit - 10 {
    log_warn(`Rate limit approaching for ${client_ip}: ${count}/${limit}`)
  }
  
  store.incr(rate_key, 1, 60)
  
  response.body = {data: "value"}
}
```

### Async Operation Logging

```httpdsl
route GET "/aggregate" {
  log_info("Starting parallel requests")
  
  f1 = async fetch("https://api1.example.com/data")
  f2 = async fetch("https://api2.example.com/data")
  f3 = async fetch("https://api3.example.com/data")
  
  log_info("Waiting for responses")
  
  r1, r2, r3 = await(f1, f2, f3)
  
  log_info("All requests completed")
  
  response.body = {
    api1: r1.body,
    api2: r2.body,
    api3: r3.body
  }
}
```

### Slow Query Detection

```httpdsl
route GET "/api/slow-query" {
  start = now()
  
  log_info("Query started")
  
  sleep(2000)
  
  end = now()
  duration = end - start
  
  if duration > 1 {
    log_warn(`Slow query detected: ${duration}s`)
  }
  
  log_info(`Query completed in ${duration}s`)
  
  response.body = {duration: duration}
}
```

### Authentication Logging

```httpdsl
route POST "/auth/login" json {
  {username, password} = request.data
  
  log_info(`Login attempt for user: ${username} from ${request.ip}`)
  
  if username == "admin" && password == "secret" {
    log_info(`Successful login: ${username}`)
    
    request.session.user_id = 1
    request.session.username = username
    
    response.body = {success: true}
  } else {
    log_warn(`Failed login attempt: ${username} from ${request.ip}`)
    
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```

### Structured Logging

```httpdsl
fn log_event(event_type, data) {
  log_entry = {
    timestamp: now(),
    type: event_type,
    data: data,
    server: server_stats().uptime_human
  }
  
  log_info(json.stringify(log_entry))
}

route POST "/api/events" json {
  event = request.data
  
  log_event("api_event", {
    event_type: event.type,
    user_id: request.session.user_id,
    ip: request.ip
  })
  
  response.body = {received: true}
}
```

### Debug Sleep

```httpdsl
route GET "/slow-response" {
  delay = int(request.query.delay ?? "0")
  
  if delay > 0 && delay <= 10 {
    log_info(`Simulating delay of ${delay} seconds`)
    sleep(delay * 1000)
  }
  
  response.body = {message: "Completed", delay: delay}
}
```

### Memory Monitoring

```httpdsl
every 5 m {
  stats = server_stats()
  
  log_info(`Server stats - Memory: ${stats.mem_alloc_mb}MB, GC cycles: ${stats.gc_count}, Uptime: ${stats.uptime_human}`)
  
  if stats.mem_alloc_mb > 512 {
    log_warn("High memory usage detected")
  }
  
  if stats.goroutines > 500 {
    log_warn(`High goroutine count: ${stats.goroutines}`)
  }
}
```

### Request Timing

```httpdsl
before {
  store.set(`req_start:${cuid2()}`, now())
}

after {
  all_keys = keys(store.all())
  
  each key in all_keys {
    if starts_with(key, "req_start:") {
      start = store.get(key)
      duration = now() - start
      
      if duration > 2 {
        log_warn(`Slow request: ${request.path} took ${duration}s`)
      }
      
      store.delete(key)
      break
    }
  }
}
```

### Shutdown Logging

```httpdsl
shutdown {
  stats = server_stats()
  
  log_info("Server shutting down...")
  log_info(`Final stats - Uptime: ${stats.uptime_human}, Total GC cycles: ${stats.gc_count}`)
  log_info("Cleanup complete")
}
```
