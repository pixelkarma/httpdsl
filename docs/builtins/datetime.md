# DateTime Builtins

Functions for date and time operations.

## date()

Get current date/time:

```httpdsl
date()
date("unix")
date("2006-01-02")
```

- `date()` - ISO 8601 string (e.g., `"2024-01-15T10:30:00Z"`)
- `date("unix")` - Unix timestamp (seconds since epoch)
- `date(format)` - Custom format (Go time format)

## date_format()

Format timestamp:

```httpdsl
ts = date("unix")
date_format(ts, "2006-01-02")
date_format(ts, "2006-01-02 15:04:05")
date_format(ts, "Mon Jan 2 15:04:05 2006")
```

## date_parse()

Parse date string:

```httpdsl
date_parse("2024-01-15", "2006-01-02")
date_parse("15/01/2024", "02/01/2006")
date_parse("Jan 15, 2024", "Jan 2, 2006")
```

Returns Unix timestamp.

## strtotime()

Relative time calculations:

```httpdsl
strtotime("+1 hour")
strtotime("-30 minutes")
strtotime("+7 days")
strtotime("-1 week")
strtotime("+1 month")
strtotime("+1 year")
```

Returns Unix timestamp.

## Complete Examples

### Timestamp API

```httpdsl
route GET "/time" {
  now = date()
  unix = date("unix")
  formatted = date("2006-01-02 15:04:05")
  
  response.body = {
    iso: now,
    unix: unix,
    formatted: formatted
  }
}
```

### Date Formatting

```httpdsl
route GET "/format-date" {
  timestamp = int(request.query.timestamp ?? str(date("unix")))
  
  response.body = {
    timestamp: timestamp,
    iso: date_format(timestamp, "2006-01-02T15:04:05Z"),
    short: date_format(timestamp, "2006-01-02"),
    long: date_format(timestamp, "Monday, January 2, 2006"),
    time: date_format(timestamp, "15:04:05")
  }
}
```

### Date Parsing

```httpdsl
route POST "/parse-date" json {
  date_str = request.data.date ?? ""
  format = request.data.format ?? "2006-01-02"
  
  try {
    timestamp = date_parse(date_str, format)
    
    response.body = {
      input: date_str,
      format: format,
      timestamp: timestamp,
      iso: date_format(timestamp, "2006-01-02T15:04:05Z")
    }
  } catch err {
    response.status = 400
    response.body = {error: "Invalid date format"}
  }
}
```

### Expiration Check

```httpdsl
route GET "/api/subscription" {
  expires_at = 1735689600
  now = date("unix")
  
  if now > expires_at {
    response.status = 403
    response.body = {
      error: "Subscription expired",
      expired_at: date_format(expires_at, "2006-01-02")
    }
    return
  }
  
  days_left = int((expires_at - now) / 86400)
  
  response.body = {
    status: "active",
    days_remaining: days_left,
    expires_at: date_format(expires_at, "2006-01-02")
  }
}
```

### Relative Time

```httpdsl
route GET "/schedule" {
  now = date("unix")
  
  one_hour_from_now = strtotime("+1 hour")
  tomorrow = strtotime("+1 day")
  next_week = strtotime("+1 week")
  
  response.body = {
    now: date_format(now, "2006-01-02 15:04:05"),
    one_hour: date_format(one_hour_from_now, "2006-01-02 15:04:05"),
    tomorrow: date_format(tomorrow, "2006-01-02 15:04:05"),
    next_week: date_format(next_week, "2006-01-02 15:04:05")
  }
}
```

### Token Expiration

```httpdsl
route POST "/api/generate-token" {
  token = cuid2()
  expires = strtotime("+1 hour")
  
  store.set(`token:${token}`, {
    created: date(),
    expires: expires
  }, 3600)
  
  response.body = {
    token: token,
    expires_at: date_format(expires, "2006-01-02 15:04:05"),
    expires_in: 3600
  }
}

route GET "/api/validate-token" {
  token = request.query.token ?? ""
  
  data = store.get(`token:${token}`)
  
  if data == null {
    response.status = 401
    response.body = {error: "Invalid token"}
    return
  }
  
  now = date("unix")
  
  if now > data.expires {
    response.status = 401
    response.body = {error: "Token expired"}
    return
  }
  
  response.body = {valid: true}
}
```

### Date Range Query

```httpdsl
route GET "/api/events" {
  start_str = request.query.start ?? ""
  end_str = request.query.end ?? ""
  
  if start_str == "" || end_str == "" {
    response.status = 400
    response.body = {error: "Start and end dates required"}
    return
  }
  
  try {
    start = date_parse(start_str, "2006-01-02")
    end = date_parse(end_str, "2006-01-02")
    
    if start > end {
      response.status = 400
      response.body = {error: "Start date must be before end date"}
      return
    }
    
    days = int((end - start) / 86400)
    
    response.body = {
      start: start_str,
      end: end_str,
      days: days
    }
  } catch err {
    response.status = 400
    response.body = {error: "Invalid date format. Use YYYY-MM-DD"}
  }
}
```

### Scheduled Events

```httpdsl
route POST "/api/schedule-event" json {
  {title, schedule_time} = request.data
  
  scheduled_ts = strtotime(schedule_time)
  now = date("unix")
  
  if scheduled_ts <= now {
    response.status = 400
    response.body = {error: "Scheduled time must be in the future"}
    return
  }
  
  event = {
    id: cuid2(),
    title: title,
    scheduled_at: scheduled_ts,
    scheduled_readable: date_format(scheduled_ts, "2006-01-02 15:04:05"),
    created_at: date()
  }
  
  response.status = 201
  response.body = event
}
```

### Age Calculation

```httpdsl
route POST "/calculate-age" json {
  birthdate = request.data.birthdate ?? ""
  
  try {
    birth_ts = date_parse(birthdate, "2006-01-02")
    now_ts = date("unix")
    
    age_seconds = now_ts - birth_ts
    age_years = int(age_seconds / 31536000)
    
    response.body = {
      birthdate: birthdate,
      age: age_years
    }
  } catch err {
    response.status = 400
    response.body = {error: "Invalid date format. Use YYYY-MM-DD"}
  }
}
```

### Activity Log with Timestamps

```httpdsl
route POST "/api/log" json {
  action = request.data.action ?? "unknown"
  
  log_entry = {
    id: cuid2(),
    action: action,
    timestamp: date("unix"),
    datetime: date(),
    user_agent: request.headers["user-agent"] ?? "Unknown",
    ip: request.ip
  }
  
  file.append("./activity.log", json.stringify(log_entry) + "\n")
  
  response.body = {logged: true}
}
```

### Cache with Expiration

```httpdsl
route GET "/api/cached-data" {
  cache_key = "data:latest"
  cached = store.get(cache_key)
  
  if cached != null {
    response.headers = {
      "X-Cache": "HIT",
      "X-Cached-At": cached.cached_at
    }
    response.body = cached.data
    return
  }
  
  data = {value: rand(100), generated: date()}
  
  store.set(cache_key, {
    data: data,
    cached_at: date()
  }, 300)
  
  response.headers = {"X-Cache": "MISS"}
  response.body = data
}
```
