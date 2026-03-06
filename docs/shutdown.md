# Shutdown Handlers

The `shutdown` block runs when the server receives SIGINT or SIGTERM signals.

## Basic Shutdown Handler

```httpdsl
server {
  port 3000
}

shutdown {
  log_info("Server shutting down...")
}
```

## Database Cleanup

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

shutdown {
  log_info("Closing database connection")
  db_conn.close()
  log_info("Database connection closed")
}
```

## Save State

```httpdsl
server {
  port 3000
}

counter = 0

route GET "/increment" {
  counter += 1
  response.body = {counter: counter}
}

shutdown {
  log_info("Saving state before shutdown")
  
  state = {
    counter: counter,
    shutdown_time: date()
  }
  
  file.write_json("./state.json", state)
  
  log_info("State saved")
}
```

## Graceful Shutdown

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

shutdown {
  log_info("Graceful shutdown initiated")
  
  log_info("Flushing store to disk")
  
  log_info("Closing database connections")
  db_conn.close()
  
  log_info("Saving final state")
  file.write("./shutdown.log", `Shutdown at ${date()}\n`)
  
  log_info("Shutdown complete")
}
```

## Notify External Services

```httpdsl
server {
  port 3000
}

shutdown {
  log_info("Notifying monitoring service")
  
  try {
    fetch("https://monitoring.example.com/shutdown", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: {
        server: "api-server",
        timestamp: date()
      }
    })
    log_info("Monitoring service notified")
  } catch err {
    log_error(`Failed to notify monitoring: ${err}`)
  }
}
```

## Close Connections

```httpdsl
server {
  port 3000
}

db_conn = db.open("postgres", env("DATABASE_URL"))
redis_conn = db.open("redis", "localhost:6379")

shutdown {
  log_info("Closing connections")
  
  try {
    db_conn.close()
    log_info("PostgreSQL connection closed")
  } catch err {
    log_error(`Failed to close PostgreSQL: ${err}`)
  }
  
  try {
    redis_conn.close()
    log_info("Redis connection closed")
  } catch err {
    log_error(`Failed to close Redis: ${err}`)
  }
  
  log_info("All connections closed")
}
```

## Broadcast Shutdown

```httpdsl
server {
  port 3000
}

route SSE "/events" {
  stream.send("connected", {message: "Connected"})
}

shutdown {
  log_info("Broadcasting shutdown notice")
  
  broadcast("shutdown", {
    message: "Server is shutting down",
    timestamp: date()
  })
  
  sleep(2000)
  
  log_info("Shutdown broadcast complete")
}
```

## Complete Cleanup Example

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

store.sync("./store.json", 60)

route GET "/" {
  visits = store.get("visits", 0)
  store.incr("visits", 1)
  response.body = {visits: visits + 1}
}

shutdown {
  log_info("=== Starting graceful shutdown ===")
  
  log_info("1. Broadcasting shutdown to connected clients")
  broadcast("shutdown", {message: "Server shutting down"})
  sleep(1000)
  
  log_info("2. Saving statistics")
  stats = server_stats()
  final_stats = {
    uptime: stats.uptime,
    total_visits: store.get("visits", 0),
    shutdown_time: date()
  }
  file.write_json("./shutdown_stats.json", final_stats)
  
  log_info("3. Flushing store to disk")
  
  log_info("4. Closing database connection")
  try {
    db_conn.close()
    log_info("Database connection closed successfully")
  } catch err {
    log_error(`Database close error: ${err}`)
  }
  
  log_info("5. Writing shutdown log")
  file.append("./app.log", `${date()}: Server shutdown\n`)
  
  log_info("=== Graceful shutdown complete ===")
}
```

## Rollback Transactions

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

in_transaction = false

route POST "/transaction/start" {
  db_conn.exec("BEGIN TRANSACTION", [])
  in_transaction = true
  response.body = {started: true}
}

route POST "/transaction/commit" {
  db_conn.exec("COMMIT", [])
  in_transaction = false
  response.body = {committed: true}
}

shutdown {
  if in_transaction {
    log_warn("Rolling back uncommitted transaction")
    try {
      db_conn.exec("ROLLBACK", [])
      log_info("Transaction rolled back")
    } catch err {
      log_error(`Rollback failed: ${err}`)
    }
  }
  
  db_conn.close()
}
```

## Cleanup Temporary Files

```httpdsl
server {
  port 3000
}

shutdown {
  log_info("Cleaning up temporary files")
  
  if file.exists("./temp") {
    temp_files = file.list("./temp")
    
    each filename in temp_files {
      try {
        file.delete(`./temp/${filename}`)
        log_info(`Deleted temp file: ${filename}`)
      } catch err {
        log_error(`Failed to delete ${filename}: ${err}`)
      }
    }
    
    log_info(`Cleaned ${len(temp_files)} temporary files`)
  }
}
```

## Lock File Removal

```httpdsl
server {
  port 3000
}

lock_file = "./server.lock"

file.write(lock_file, str(date("unix")))

shutdown {
  log_info("Removing lock file")
  
  if file.exists(lock_file) {
    file.delete(lock_file)
    log_info("Lock file removed")
  }
}
```

## Auto-Flush Behavior

The server automatically flushes:
- Store (if sync is configured)
- Session store (if sync is configured)

Your shutdown handler runs before these auto-flushes.

## Shutdown Timeout

Shutdown handlers should complete quickly. Long-running operations may be interrupted by the OS.

## Testing Shutdown

Trigger shutdown with Ctrl+C or:

```bash
kill -SIGTERM <pid>
```

## Production Example

```httpdsl
server {
  port 3000
}

is_production = env("ENV") == "production"

db_conn = db.open("postgres", env("DATABASE_URL", "postgres://localhost/myapp"))

store.sync(db_conn, "kv_store", 30)

shutdown {
  timestamp = date()
  
  log_info(`[${timestamp}] Graceful shutdown initiated`)
  
  if is_production {
    log_info("Notifying load balancer")
    try {
      fetch(env("LB_HEALTH_URL"), {
        method: "POST",
        body: {status: "shutting_down"}
      })
    } catch err {
      log_error(`LB notification failed: ${err}`)
    }
    
    log_info("Waiting for active requests to complete")
    sleep(5000)
  }
  
  log_info("Broadcasting shutdown to clients")
  broadcast("server_shutdown", {
    message: "Server is shutting down",
    timestamp: timestamp
  })
  
  log_info("Persisting final metrics")
  stats = server_stats()
  metrics = {
    timestamp: timestamp,
    uptime: stats.uptime,
    memory_mb: stats.mem_alloc_mb,
    goroutines: stats.goroutines
  }
  file.write_json("./metrics/shutdown.json", metrics)
  
  log_info("Closing database")
  db_conn.close()
  
  log_info(`[${date()}] Shutdown complete`)
}
```
