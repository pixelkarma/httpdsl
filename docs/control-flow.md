# Control Flow

HTTPDSL provides standard control flow constructs for conditionals and loops.

## If Statement

```httpdsl
if condition {
  response.body = "True branch"
}
```

With else:

```httpdsl
if age >= 18 {
  response.body = "Adult"
} else {
  response.body = "Minor"
}
```

Else if:

```httpdsl
if score >= 90 {
  grade = "A"
} else if score >= 80 {
  grade = "B"
} else if score >= 70 {
  grade = "C"
} else {
  grade = "F"
}
```

## Switch Statement

Pattern matching without fallthrough:

```httpdsl
switch method {
  case "GET" {
    response.body = "Reading data"
  }
  case "POST" {
    response.body = "Creating data"
  }
  case "PUT" {
    response.body = "Updating data"
  }
  default {
    response.body = "Unknown method"
  }
}
```

With expressions:

```httpdsl
route GET "/api/status" {
  code = int(request.query.code ?? "200")
  
  switch code {
    case 200 {
      message = "OK"
    }
    case 201 {
      message = "Created"
    }
    case 400 {
      message = "Bad Request"
    }
    case 404 {
      message = "Not Found"
    }
    case 500 {
      message = "Internal Server Error"
    }
    default {
      message = "Unknown status"
    }
  }
  
  response.body = {code: code, message: message}
}
```

**Note**: Cases do not fall through. Each case is independent.

## While Loop

Repeat while condition is true:

```httpdsl
i = 0
sum = 0

while i < 10 {
  sum += i
  i += 1
}
```

With break:

```httpdsl
i = 0

while true {
  if i >= 10 {
    break
  }
  i += 1
}
```

With continue:

```httpdsl
i = 0
sum = 0

while i < 10 {
  i += 1
  
  if i % 2 == 0 {
    continue
  }
  
  sum += i
}
```

## Each Loop

Iterate over arrays:

```httpdsl
fruits = ["apple", "banana", "cherry"]

each fruit in fruits {
  log(fruit)
}
```

Iterate over ranges:

```httpdsl
each i in range(5) {
  log(i)
}

each i in range(1, 10) {
  log(i)
}

each i in range(0, 100, 10) {
  log(i)
}
```

With break:

```httpdsl
numbers = [1, 2, 3, 4, 5]
found = null

each num in numbers {
  if num == 3 {
    found = num
    break
  }
}
```

With continue:

```httpdsl
numbers = [1, 2, 3, 4, 5]
sum = 0

each num in numbers {
  if num % 2 == 0 {
    continue
  }
  sum += num
}
```

## Break Statement

Exit a loop early:

```httpdsl
each item in items {
  if item == "stop" {
    break
  }
  log(item)
}
```

## Continue Statement

Skip to the next iteration:

```httpdsl
each num in range(10) {
  if num % 2 == 0 {
    continue
  }
  log(num)
}
```

## Practical Examples

### Request Validation

```httpdsl
route POST "/api/users" json {
  {name, email, age} = request.data
  errors = []
  
  if name == "" || name == null {
    errors = append(errors, "Name is required")
  }
  
  if email == "" || email == null {
    errors = append(errors, "Email is required")
  } else if !is_email(email) {
    errors = append(errors, "Invalid email format")
  }
  
  if age == null {
    errors = append(errors, "Age is required")
  } else if age < 18 {
    errors = append(errors, "Must be 18 or older")
  }
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
    return
  }
  
  response.status = 201
  response.body = {id: cuid2(), name: name, email: email, age: age}
}
```

### Search Filter

```httpdsl
route GET "/api/users/search" {
  query = lower(request.query.q ?? "")
  
  all_users = [
    {id: 1, name: "Alice", role: "admin"},
    {id: 2, name: "Bob", role: "user"},
    {id: 3, name: "Charlie", role: "user"},
    {id: 4, name: "Diana", role: "moderator"}
  ]
  
  results = []
  
  each user in all_users {
    if query == "" || contains(lower(user.name), query) {
      results = append(results, user)
    }
  }
  
  response.body = {results: results, count: len(results)}
}
```

### Role-Based Permission Check

```httpdsl
allowed_roles = ["admin", "moderator", "editor"]
user_role = request.session.role ?? "guest"
has_permission = false

each role in allowed_roles {
  if role == user_role {
    has_permission = true
    break
  }
}

if !has_permission {
  response.status = 403
  response.body = {error: "Access denied"}
  return
}
```

### Processing Items with Limits

```httpdsl
route POST "/api/batch" json {
  items = request.data.items ?? []
  max_items = 100
  
  if len(items) > max_items {
    response.status = 400
    response.body = {error: `Too many items. Maximum is ${max_items}`}
    return
  }
  
  results = []
  
  each item in items {
    if item.id == null {
      continue
    }
    
    processed = {
      id: item.id,
      status: "processed",
      timestamp: now()
    }
    
    results = append(results, processed)
  }
  
  response.body = {processed: len(results), results: results}
}
```

### Finding First Match

```httpdsl
route GET "/api/find/:id" {
  target_id = int(request.params.id)
  
  items = [
    {id: 1, name: "Item 1"},
    {id: 2, name: "Item 2"},
    {id: 3, name: "Item 3"}
  ]
  
  found = null
  
  each item in items {
    if item.id == target_id {
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
```

### State Machine

```httpdsl
route POST "/api/workflow" json {
  state = request.data.state ?? "idle"
  action = request.data.action ?? ""
  
  switch state {
    case "idle" {
      switch action {
        case "start" {
          new_state = "running"
        }
        default {
          new_state = "idle"
        }
      }
    }
    case "running" {
      switch action {
        case "pause" {
          new_state = "paused"
        }
        case "stop" {
          new_state = "stopped"
        }
        default {
          new_state = "running"
        }
      }
    }
    case "paused" {
      switch action {
        case "resume" {
          new_state = "running"
        }
        case "stop" {
          new_state = "stopped"
        }
        default {
          new_state = "paused"
        }
      }
    }
    default {
      new_state = "idle"
    }
  }
  
  response.body = {old_state: state, action: action, new_state: new_state}
}
```
