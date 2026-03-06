# DateTime Builtins

Functions for date and time operations.

## now()

Get current unix timestamp in seconds:

```httpdsl
now()
```

Returns the current time as an `int64` unix timestamp (seconds since epoch).

Equivalent to `date().unix`.

## now_ms()

Get current unix timestamp in milliseconds:

```httpdsl
now_ms()
```

Returns the current time as an `int64` unix timestamp in milliseconds.

## date()

Get date/time components as a map:

```httpdsl
date()
date(1705312200)
```

- `date()` — returns a map of the current date/time
- `date(unixTimestamp)` — returns a map for the given unix timestamp

The returned map contains these `int64` fields:

| Field     | Description                            |
|-----------|----------------------------------------|
| `year`    | Four-digit year (e.g., `2024`)         |
| `month`   | Month of the year (`1`–`12`)           |
| `day`     | Day of the month (`1`–`31`)            |
| `hour`    | Hour (`0`–`23`)                        |
| `minute`  | Minute (`0`–`59`)                      |
| `second`  | Second (`0`–`59`)                      |
| `weekday` | Day of week (`0`=Sunday, `6`=Saturday) |
| `unix`    | Unix timestamp in seconds              |

```httpdsl
d = date()
d.year    // e.g., 2024
d.month   // e.g., 1
d.day     // e.g., 15
d.hour    // e.g., 10
d.unix    // e.g., 1705312200
```

> **Note:** `date()` does not accept a format string. Passing a string like `date("unix")` will coerce the string to `0` and return the epoch (Jan 1, 1970). Use `now()` or `date().unix` to get a unix timestamp.

## date_format()

Format a unix timestamp as a string:

```httpdsl
date_format(unixTimestamp, formatString)
```

Takes a unix timestamp (`int64`) and a Go time format string. Returns a formatted string.

Go format strings use the reference time `Mon Jan 2 15:04:05 MST 2006`:

| Component | Reference value |
|-----------|-----------------|
| Year      | `2006`          |
| Month     | `01` or `Jan`   |
| Day       | `02`            |
| Hour (24) | `15`            |
| Minute    | `04`            |
| Second    | `05`            |
| Weekday   | `Mon`           |
| Timezone  | `MST`           |

```httpdsl
ts = now()
date_format(ts, "2006-01-02")              // "2024-01-15"
date_format(ts, "2006-01-02 15:04:05")     // "2024-01-15 10:30:00"
date_format(ts, "Mon Jan 2 15:04:05 2006") // "Mon Jan 15 10:30:00 2024"
date_format(ts, "15:04:05")                // "10:30:00"
```

Returns an empty string if fewer than 2 arguments are provided.

## date_parse()

Parse a date string into a unix timestamp:

```httpdsl
date_parse(dateString, formatString)
```

Takes a date string and a Go format string. Returns a unix timestamp (`int64`), or `null` on parse failure.

```httpdsl
date_parse("2024-01-15", "2006-01-02")                // 1705276800
date_parse("15/01/2024", "02/01/2006")                // 1705276800
date_parse("Jan 15, 2024", "Jan 2, 2006")             // 1705276800
date_parse("2024-01-15 10:30:00", "2006-01-02 15:04:05") // 1705314600
```

Returns `null` if the string does not match the format or if fewer than 2 arguments are provided.

## strtotime()

Parse relative time expressions:

```httpdsl
strtotime(expression)
```

Returns a unix timestamp (`int64`).

Supported expression forms:

| Form                       | Example                     | Description                                |
|----------------------------|-----------------------------|--------------------------------------------|
| `"now"`                    | `strtotime("now")`          | Current unix timestamp                     |
| `"now + N unit"`           | `strtotime("now + 3 days")` | Current time plus offset                   |
| `"now - N unit"`           | `strtotime("now - 2 hours")`| Current time minus offset                  |
| `"+N unit"`                | `strtotime("+3 days")`      | Current time plus offset                   |
| `"-N unit"`                | `strtotime("-2 hours")`     | Current time minus offset                  |
| `"timestamp + N unit"`     | `strtotime("1705312200 + 2 hours")` | Base timestamp plus offset        |
| `"timestamp - N unit"`     | `strtotime("1705312200 - 1 day")`   | Base timestamp minus offset       |

Supported time units (singular or plural):

- `second` / `seconds`
- `minute` / `minutes`
- `hour` / `hours`
- `day` / `days`
- `week` / `weeks`
- `month` / `months`
- `year` / `years`

```httpdsl
strtotime("now")                    // current unix timestamp
strtotime("+1 hour")                // one hour from now
strtotime("-30 minutes")            // 30 minutes ago
strtotime("+7 days")                // one week from now
strtotime("-1 week")                // one week ago
strtotime("+1 month")               // one month from now
strtotime("+1 year")                // one year from now
strtotime("now + 3 days")           // 3 days from now
strtotime("1705312200 + 2 hours")   // 2 hours after the given timestamp
```

Returns `null` if no arguments are provided.

## Complete Examples

### Timestamp API

```httpdsl
route GET "/time" {
  d = date()

  response.body = {
    unix: d.unix,
    year: d.year,
    month: d.month,
    day: d.day,
    hour: d.hour,
    minute: d.minute,
    second: d.second,
    formatted: date_format(d.unix, "2006-01-02 15:04:05")
  }
}
```

### Date Formatting

```httpdsl
route GET "/format-date" {
  timestamp = int(request.query.timestamp ?? str(now()))

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

  timestamp = date_parse(date_str, format)

  if timestamp == null {
    response.status = 400
    response.body = {error: "Invalid date format"}
    return
  }

  response.body = {
    input: date_str,
    format: format,
    timestamp: timestamp,
    iso: date_format(timestamp, "2006-01-02T15:04:05Z")
  }
}
```

### Expiration Check

```httpdsl
route GET "/api/subscription" {
  expires_at = 1735689600
  current = now()

  if current > expires_at {
    response.status = 403
    response.body = {
      error: "Subscription expired",
      expired_at: date_format(expires_at, "2006-01-02")
    }
    return
  }

  days_left = int((expires_at - current) / 86400)

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
  current = now()

  one_hour_from_now = strtotime("+1 hour")
  tomorrow = strtotime("+1 day")
  next_week = strtotime("+1 week")

  response.body = {
    now: date_format(current, "2006-01-02 15:04:05"),
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

  store.set("token:" + token, {
    created: now(),
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

  data = store.get("token:" + token)

  if data == null {
    response.status = 401
    response.body = {error: "Invalid token"}
    return
  }

  if now() > data.expires {
    response.status = 401
    response.body = {error: "Token expired"}
    return
  }

  response.body = {valid: true}
}
```

### Millisecond Timestamps

```httpdsl
route GET "/api/ping" {
  start = now_ms()

  // ... do some work ...

  elapsed = now_ms() - start

  response.body = {
    timestamp_ms: now_ms(),
    elapsed_ms: elapsed
  }
}
```

### Date Components

```httpdsl
route GET "/api/today" {
  d = date()

  weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"]

  response.body = {
    year: d.year,
    month: d.month,
    day: d.day,
    weekday: weekdays[d.weekday],
    is_weekend: d.weekday == 0 || d.weekday == 6
  }
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

  start = date_parse(start_str, "2006-01-02")
  end = date_parse(end_str, "2006-01-02")

  if start == null || end == null {
    response.status = 400
    response.body = {error: "Invalid date format. Use YYYY-MM-DD"}
    return
  }

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
}
```

### Historical Date Lookup

```httpdsl
route GET "/api/date-info" {
  ts = int(request.query.timestamp ?? "0")
  d = date(ts)

  response.body = {
    timestamp: ts,
    formatted: date_format(ts, "2006-01-02 15:04:05"),
    year: d.year,
    month: d.month,
    day: d.day,
    hour: d.hour,
    minute: d.minute,
    second: d.second,
    weekday: d.weekday
  }
}
```
