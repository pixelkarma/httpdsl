# Validation Builtins

Functions for data validation.

## validate()

Validate data against schema:

```httpdsl
schema = {
  name: {required: true, type: "string", min: 3, max: 50},
  email: {required: true, type: "string", format: "email"},
  age: {required: false, type: "number", min: 18, max: 120}
}

data = {name: "Alice", email: "alice@example.com", age: 30}
errors = validate(data, schema)
```

Returns array of error strings (empty if valid).

### Schema Fields

- `required`: `true`/`false`
- `type`: `"string"`, `"number"`, `"bool"`, `"array"`
- `min`: Minimum value/length
- `max`: Maximum value/length
- `format`: `"email"`, `"url"`, `"uuid"`
- `in`: Array of allowed values
- `regex`: Regular expression pattern

## is_email()

Check if string is valid email:

```httpdsl
is_email("alice@example.com")
is_email("invalid-email")
is_email("")
```

## is_url()

Check if string is valid URL:

```httpdsl
is_url("https://example.com")
is_url("http://localhost:3000")
is_url("not-a-url")
```

## is_uuid()

Check if string is valid UUID:

```httpdsl
is_uuid("550e8400-e29b-41d4-a716-446655440000")
is_uuid("invalid-uuid")
```

## is_numeric()

Check if string contains only numbers:

```httpdsl
is_numeric("12345")
is_numeric("123.45")
is_numeric("abc123")
```

## Complete Examples

### User Registration

```httpdsl
route POST "/auth/register" json {
  data = request.data
  
  schema = {
    username: {
      required: true,
      type: "string",
      min: 3,
      max: 20
    },
    email: {
      required: true,
      type: "string",
      format: "email"
    },
    password: {
      required: true,
      type: "string",
      min: 8
    },
    age: {
      required: true,
      type: "number",
      min: 18
    }
  }
  
  errors = validate(data, schema)
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
    return
  }
  
  response.status = 201
  response.body = {message: "Registration successful"}
}
```

### Product Creation

```httpdsl
route POST "/api/products" json {
  data = request.data
  
  schema = {
    name: {
      required: true,
      type: "string",
      min: 1,
      max: 100
    },
    price: {
      required: true,
      type: "number",
      min: 0
    },
    category: {
      required: true,
      type: "string",
      in: ["electronics", "clothing", "food"]
    },
    sku: {
      required: true,
      type: "string",
      regex: "^[A-Z0-9]{8}$"
    }
  }
  
  errors = validate(data, schema)
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
    return
  }
  
  product = {
    id: cuid2(),
    name: data.name,
    price: data.price,
    category: data.category,
    sku: data.sku
  }
  
  response.status = 201
  response.body = product
}
```

### Email Validation

```httpdsl
route POST "/validate-email" json {
  email = request.data.email ?? ""
  
  if email == "" {
    response.status = 400
    response.body = {error: "Email is required"}
    return
  }
  
  if !is_email(email) {
    response.status = 400
    response.body = {error: "Invalid email format"}
    return
  }
  
  response.body = {valid: true, email: email}
}
```

### URL Validation

```httpdsl
route POST "/shorten-url" json {
  url = request.data.url ?? ""
  
  if !is_url(url) {
    response.status = 400
    response.body = {error: "Invalid URL"}
    return
  }
  
  short_id = cuid2(8)
  store.set(`url:${short_id}`, url)
  
  response.body = {
    original: url,
    short: `/s/${short_id}`
  }
}
```

### Settings Update

```httpdsl
route PUT "/settings" json {
  data = request.data
  
  schema = {
    theme: {
      required: false,
      type: "string",
      in: ["light", "dark", "auto"]
    },
    language: {
      required: false,
      type: "string",
      in: ["en", "es", "fr", "de"]
    },
    notifications: {
      required: false,
      type: "bool"
    },
    email_frequency: {
      required: false,
      type: "string",
      in: ["daily", "weekly", "monthly", "never"]
    }
  }
  
  errors = validate(data, schema)
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
    return
  }
  
  response.body = {updated: true, settings: data}
}
```

### Custom Validation

```httpdsl
fn validate_username(username) {
  if username == "" {
    return "Username is required"
  }
  
  if len(username) < 3 {
    return "Username must be at least 3 characters"
  }
  
  if len(username) > 20 {
    return "Username must be at most 20 characters"
  }
  
  if !is_numeric(slice(username, 0, 1)) {
    if starts_with(username, "0") {
      return "Username cannot start with a number"
    }
  }
  
  if contains(username, " ") {
    return "Username cannot contain spaces"
  }
  
  return null
}

route POST "/validate-username" json {
  username = request.data.username ?? ""
  
  error = validate_username(username)
  
  if error != null {
    response.status = 400
    response.body = {error: error}
    return
  }
  
  response.body = {valid: true}
}
```

### Multi-Field Validation

```httpdsl
route POST "/api/contact" json {
  {name, email, phone, message} = request.data
  
  errors = []
  
  if name == "" || name == null {
    errors = append(errors, "Name is required")
  } else if len(name) < 2 {
    errors = append(errors, "Name must be at least 2 characters")
  }
  
  if email == "" || email == null {
    errors = append(errors, "Email is required")
  } else if !is_email(email) {
    errors = append(errors, "Invalid email format")
  }
  
  if phone != "" && phone != null {
    if !is_numeric(replace(phone, "-", "")) {
      errors = append(errors, "Phone must contain only numbers and dashes")
    }
  }
  
  if message == "" || message == null {
    errors = append(errors, "Message is required")
  } else if len(message) < 10 {
    errors = append(errors, "Message must be at least 10 characters")
  }
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
    return
  }
  
  response.body = {message: "Contact form submitted"}
}
```

### Array Validation

```httpdsl
route POST "/api/bulk-create" json {
  items = request.data.items ?? []
  
  schema = {
    name: {required: true, type: "string", min: 1},
    price: {required: true, type: "number", min: 0}
  }
  
  all_errors = {}
  
  i = 0
  each item in items {
    errors = validate(item, schema)
    
    if len(errors) > 0 {
      all_errors[str(i)] = errors
    }
    
    i += 1
  }
  
  if len(keys(all_errors)) > 0 {
    response.status = 400
    response.body = {errors: all_errors}
    return
  }
  
  response.status = 201
  response.body = {created: len(items)}
}
```
