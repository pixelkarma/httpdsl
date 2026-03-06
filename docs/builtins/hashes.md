# Hash Builtins

Functions for hash (object/dictionary) manipulation.

## keys()

Get all keys as array:

```httpdsl
keys({name: "Alice", age: 30})
keys({a: 1, b: 2, c: 3})
keys({})
```

## values()

Get all values as array:

```httpdsl
values({name: "Alice", age: 30})
values({a: 1, b: 2, c: 3})
values({})
```

## merge()

Merge two hashes:

```httpdsl
merge({a: 1, b: 2}, {c: 3, d: 4})
merge({name: "Alice"}, {age: 30})
merge({x: 1}, {x: 2})
```

Later values override earlier ones:

```httpdsl
defaults = {theme: "light", lang: "en"}
user_prefs = {theme: "dark"}
final = merge(defaults, user_prefs)
```

## delete()

Remove key from hash:

```httpdsl
data = {a: 1, b: 2, c: 3}
delete(data, "b")
```

Returns modified hash:

```httpdsl
user = {id: 1, password: "secret", name: "Alice"}
clean = delete(user, "password")
```

## has()

Check if hash has key:

```httpdsl
has({name: "Alice", age: 30}, "name")
has({a: 1, b: 2}, "c")
has({}, "key")
```

## Complete Examples

### Configuration Merging

```httpdsl
default_config = {
  theme: "light",
  language: "en",
  notifications: true,
  page_size: 10
}

route GET "/config" {
  user_config = {
    theme: request.session.theme ?? null,
    language: request.session.language ?? null
  }
  
  clean_user = {}
  
  each key in keys(user_config) {
    value = user_config[key]
    if value != null {
      clean_user[key] = value
    }
  }
  
  final_config = merge(default_config, clean_user)
  
  response.body = final_config
}
```

### Object Filtering

```httpdsl
route POST "/filter-object" json {
  data = request.data.object ?? {}
  allowed_keys = request.data.allowed ?? []
  
  filtered = {}
  
  each key in allowed_keys {
    if has(data, key) {
      filtered[key] = data[key]
    }
  }
  
  response.body = filtered
}
```

### Removing Sensitive Data

```httpdsl
fn sanitize_user(user) {
  clean = user
  clean = delete(clean, "password")
  clean = delete(clean, "password_hash")
  clean = delete(clean, "secret_token")
  return clean
}

route GET "/users/:id" {
  user = {
    id: 1,
    name: "Alice",
    email: "alice@example.com",
    password: "hashed_password",
    secret_token: "token123"
  }
  
  response.body = sanitize_user(user)
}
```

### Object Validation

```httpdsl
route POST "/validate" json {
  data = request.data
  
  required_keys = ["name", "email", "age"]
  errors = []
  
  each key in required_keys {
    if !has(data, key) {
      errors = append(errors, `Missing field: ${key}`)
    } else if data[key] == "" || data[key] == null {
      errors = append(errors, `Empty field: ${key}`)
    }
  }
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
  } else {
    response.body = {valid: true}
  }
}
```

### Object Transformation

```httpdsl
route POST "/transform" json {
  input = request.data ?? {}
  
  output = {}
  
  each key in keys(input) {
    new_key = upper(key)
    output[new_key] = input[key]
  }
  
  response.body = output
}
```

### Default Values

```httpdsl
route POST "/create-user" json {
  defaults = {
    role: "user",
    active: true,
    notifications: true
  }
  
  user_data = request.data
  
  final_user = merge(defaults, user_data)
  final_user.id = cuid2()
  final_user.created_at = date()
  
  response.status = 201
  response.body = final_user
}
```

### Object Diff

```httpdsl
route POST "/diff" json {
  obj1 = request.data.obj1 ?? {}
  obj2 = request.data.obj2 ?? {}
  
  all_keys = unique(append(keys(obj1), keys(obj2)))
  
  added = []
  removed = []
  changed = []
  
  each key in all_keys {
    has_in_1 = has(obj1, key)
    has_in_2 = has(obj2, key)
    
    if !has_in_1 && has_in_2 {
      added = append(added, key)
    } else if has_in_1 && !has_in_2 {
      removed = append(removed, key)
    } else if obj1[key] != obj2[key] {
      changed = append(changed, key)
    }
  }
  
  response.body = {
    added: added,
    removed: removed,
    changed: changed
  }
}
```

### Nested Object Access

```httpdsl
route GET "/nested" {
  data = {
    user: {
      name: "Alice",
      address: {
        city: "Boston",
        state: "MA"
      }
    }
  }
  
  user_keys = keys(data.user)
  address_keys = keys(data.user.address)
  
  response.body = {
    user_keys: user_keys,
    address_keys: address_keys,
    city: data.user.address.city
  }
}
```

### Object to Array

```httpdsl
route POST "/to-array" json {
  obj = request.data ?? {}
  
  array = []
  
  each key in keys(obj) {
    item = {
      key: key,
      value: obj[key]
    }
    array = append(array, item)
  }
  
  response.body = {array: array}
}
```

### Picking Fields

```httpdsl
fn pick(obj, fields) {
  result = {}
  
  each field in fields {
    if has(obj, field) {
      result[field] = obj[field]
    }
  }
  
  return result
}

route GET "/user/:id" {
  user = {
    id: 1,
    name: "Alice",
    email: "alice@example.com",
    password_hash: "...",
    role: "admin",
    created_at: "2024-01-01"
  }
  
  public_fields = ["id", "name", "role"]
  
  response.body = pick(user, public_fields)
}
```

### Omitting Fields

```httpdsl
fn omit(obj, fields) {
  result = obj
  
  each field in fields {
    result = delete(result, field)
  }
  
  return result
}

route GET "/settings" {
  settings = {
    theme: "dark",
    language: "en",
    api_key: "secret123",
    webhook_secret: "secret456"
  }
  
  sensitive = ["api_key", "webhook_secret"]
  
  response.body = omit(settings, sensitive)
}
```
