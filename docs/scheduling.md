# Scheduling

HTTPDSL provides task scheduling with interval-based and cron-based schedules.

## Interval Scheduling

### Seconds

```httpdsl
every 5 s {
  log_info("Running every 5 seconds")
}
```

### Minutes

```httpdsl
every 1 m {
  log_info("Running every minute")
}

every 15 m {
  log_info("Running every 15 minutes")
}
```

### Hours

```httpdsl
every 1 h {
  log_info("Running every hour")
}

every 6 h {
  log_info("Running every 6 hours")
}
```

## Cron Scheduling

Use cron syntax (5 fields: minute, hour, day-of-month, month, day-of-week):

```httpdsl
every "0 * * * *" {
  log_info("Running at the start of every hour")
}

every "30 2 * * *" {
  log_info("Running at 2:30 AM every day")
}

every "0 0 * * 0" {
  log_info("Running at midnight every Sunday")
}
```

### Cron Syntax

- `*` - Any value
- `1-5` - Range
- `*/5` - Steps
- `1,3,5` - List

Examples:

```httpdsl
every "*/5 * * * *" {
  log_info("Every 5 minutes")
}

every "0 9-17 * * 1-5" {
  log_info("Every hour from 9 AM to 5 PM on weekdays")
}

every "0 0 1 * *" {
  log_info("First day of every month at midnight")
}

every "0 0 * * 1" {
  log_info("Every Monday at midnight")
}
```

## Cleanup Tasks

```httpdsl
server {
  port 3000
}

every 1 h {
  log_info("Running cleanup task")
  
  expired_keys = []
  all_keys = keys(store.all())
  
  each key in all_keys {
    if starts_with(key, "temp:") {
      expired_keys = append(expired_keys, key)
    }
  }
  
  each key in expired_keys {
    store.delete(key)
  }
  
  log_info(`Cleaned up ${len(expired_keys)} temporary keys`)
}
```

## Database Backup

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

every "0 2 * * *" {
  log_info("Starting database backup")
  
  users = db_conn.query("SELECT * FROM users", [])
  posts = db_conn.query("SELECT * FROM posts", [])
  
  backup = {
    timestamp: now(),
    users: users,
    posts: posts
  }
  
  filename = `backup_${now()}.json`
  
  if !file.exists("./backups") {
    file.mkdir("./backups")
  }
  
  file.write_json(`./backups/${filename}`, backup)
  
  log_info(`Backup saved to ${filename}`)
}
```

## Health Check

```httpdsl
server {
  port 3000
}

every 30 s {
  stats = server_stats()
  
  if stats.mem_alloc_mb > 500 {
    log_warn(`High memory usage: ${stats.mem_alloc_mb} MB`)
  }
  
  if stats.goroutines > 1000 {
    log_warn(`High goroutine count: ${stats.goroutines}`)
  }
}
```

## API Sync

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

every 5 m {
  log_info("Syncing with external API")
  
  result = fetch("https://api.example.com/data")
  
  if result.status == 200 {
    data = result.body
    
    each item in data.items {
      db_conn.exec(
        "INSERT OR REPLACE INTO cache (key, value) VALUES (?, ?)",
        [item.id, json.stringify(item)]
      )
    }
    
    log_info(`Synced ${len(data.items)} items`)
  } else {
    log_error(`Sync failed with status ${result.status}`)
  }
}
```

## Notification Sender

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

every 1 m {
  pending = db_conn.query(
    "SELECT * FROM notifications WHERE sent = 0 AND scheduled_at <= ?",
    [now()]
  )
  
  each notification in pending {
    try {
      result = fetch("https://api.email.com/send", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: {
          to: notification.email,
          subject: notification.subject,
          body: notification.body
        }
      })
      
      if result.status == 200 {
        db_conn.exec(
          "UPDATE notifications SET sent = 1 WHERE id = ?",
          [notification.id]
        )
        log_info(`Sent notification ${notification.id}`)
      }
    } catch(err) {
      log_error(`Failed to send notification ${notification.id}: ${err}`)
    }
  }
}
```

## Cache Warming

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

every 10 m {
  log_info("Warming cache")
  
  popular_items = db_conn.query(
    "SELECT * FROM items ORDER BY views DESC LIMIT 100",
    []
  )
  
  each item in popular_items {
    cache_key = `item:${item.id}`
    store.set(cache_key, item, 600)
  }
  
  log_info(`Cached ${len(popular_items)} popular items`)
}
```

## Report Generation

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

every "0 8 * * 1" {
  log_info("Generating weekly report")
  
  start_date = strtotime("-7 days")
  
  user_count = db_conn.query_value(
    "SELECT COUNT(*) FROM users WHERE created_at >= ?",
    [start_date]
  )
  
  post_count = db_conn.query_value(
    "SELECT COUNT(*) FROM posts WHERE created_at >= ?",
    [start_date]
  )
  
  report = {
    week: now(),
    new_users: user_count,
    new_posts: post_count
  }
  
  file.write_json(`./reports/weekly_${now()}.json`, report)
  
  log_info(`Weekly report generated: ${user_count} users, ${post_count} posts`)
}
```

## Session Cleanup

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 24 h
    secret env("SESSION_SECRET")
  }
}

db_conn = db.open("sqlite", "./sessions.db")

every 1 h {
  log_info("Cleaning expired sessions")
  
  result = db_conn.exec(
    "DELETE FROM sessions WHERE expires_at < ?",
    [now()]
  )
  
  log_info(`Deleted ${result.rows_affected} expired sessions`)
}
```

## Rate Limit Reset

```httpdsl
server {
  port 3000
}

every 1 h {
  log_info("Resetting rate limits")
  
  all_keys = keys(store.all())
  rate_limit_keys = []
  
  each key in all_keys {
    if starts_with(key, "rate:") {
      rate_limit_keys = append(rate_limit_keys, key)
    }
  }
  
  each key in rate_limit_keys {
    store.delete(key)
  }
  
  log_info(`Reset ${len(rate_limit_keys)} rate limit counters`)
}
```

## Heartbeat Broadcasting

```httpdsl
server {
  port 3000
}

route SSE "/events" {
  stream.send("connected", {message: "Connected"})
}

every 30 s {
  broadcast("heartbeat", {
    timestamp: now(),
    uptime: server_stats().uptime_human
  })
}
```

## Multiple Scheduled Tasks

```httpdsl
server {
  port 3000
}

every 1 m {
  log_info("Task 1: Running every minute")
}

every 5 m {
  log_info("Task 2: Running every 5 minutes")
}

every 1 h {
  log_info("Task 3: Running every hour")
}

every "0 0 * * *" {
  log_info("Task 4: Running daily at midnight")
}

every "0 12 * * *" {
  log_info("Task 5: Running daily at noon")
}
```

## Dynamic Scheduling with Store

```httpdsl
server {
  port 3000
}

store.set("last_sync", now())

every 1 m {
  last_sync = store.get("last_sync", 0)
  now = now()
  
  if now - last_sync >= 300 {
    log_info("Running sync task")
    
    store.set("last_sync", now)
  }
}

route POST "/admin/trigger-sync" {
  store.set("last_sync", 0)
  response.body = {message: "Sync will run on next schedule"}
}
```
