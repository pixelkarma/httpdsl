# Type Builtins

Functions for type checking and conversion.

## type()

Get the type of a value:

```httpdsl
type("hello")
type(42)
type(3.14)
type(true)
type(null)
type([1, 2, 3])
type({name: "Alice"})
type(fn() {})
```

Returns: `"string"`, `"int"`, `"float"`, `"bool"`, `"null"`, `"array"`, `"object"`, `"unknown"`

## str()

Convert to string:

```httpdsl
str(42)
str(3.14)
str(true)
str([1, 2, 3])
str({name: "Alice"})
```

## int()

Convert to integer:

```httpdsl
int("42")
int("123")
int(3.14)
int(3.99)
int(true)
int(false)
```

## float()

Convert to float:

```httpdsl
float("3.14")
float("10.5")
float(42)
float("123")
```

## bool()

Convert to boolean:

```httpdsl
bool(1)
bool(0)
bool("")
bool("hello")
bool(null)
bool([])
```

Falsy values: `false`, `null`, `0`, `0.0`, `""`

Truthy: everything else

## Complete Examples

### Type Validation

```httpdsl
route POST "/api/data" json {
  value = request.data.value
  
  value_type = type(value)
  
  if value_type != "string" && value_type != "int" {
    response.status = 400
    response.body = {
      error: "Value must be string or integer",
      received_type: value_type
    }
    return
  }
  
  response.body = {type: value_type, value: value}
}
```

### Query Parameter Conversion

```httpdsl
route GET "/calculate" {
  a = int(request.query.a ?? "0")
  b = int(request.query.b ?? "0")
  
  sum = a + b
  product = a * b
  
  response.body = {
    a: a,
    b: b,
    sum: sum,
    product: product
  }
}
```

### Type-Based Processing

```httpdsl
route POST "/process" json {
  input = request.data.input
  
  result = null
  
  switch type(input) {
    case "string" {
      result = upper(input)
    }
    case "int" {
      result = input * 2
    }
    case "array" {
      result = len(input)
    }
    case "object" {
      result = keys(input)
    }
    default {
      response.status = 400
      response.body = {error: "Unsupported type"}
      return
    }
  }
  
  response.body = {result: result}
}
```

### Safe Type Conversion

```httpdsl
fn safe_int(value, default_val) {
  if type(value) == "int" {
    return value
  }
  
  if type(value) == "string" {
    try {
      return int(value)
    } catch(err) {
      return default_val
    }
  }
  
  return default_val
}

route GET "/items" {
  page = safe_int(request.query.page, 1)
  limit = safe_int(request.query.limit, 10)
  
  response.body = {
    page: page,
    limit: limit
  }
}
```

### Type Coercion

```httpdsl
route POST "/coerce" json {
  data = request.data
  
  coerced = {
    as_string: str(data.value),
    as_int: int(data.value),
    as_float: float(data.value),
    as_bool: bool(data.value)
  }
  
  response.body = coerced
}
```

### Boolean Conversion

```httpdsl
route GET "/settings" {
  notifications = request.query.notifications ?? "true"
  
  enabled = notifications == "true" || notifications == "1"
  
  response.body = {
    notifications: enabled,
    raw_value: notifications
  }
}
```

### Float Precision

```httpdsl
route GET "/calculate-price" {
  price = float(request.query.price ?? "0")
  tax_rate = float(request.query.tax ?? "0.1")
  
  tax = price * tax_rate
  total = price + tax
  
  response.body = {
    price: price,
    tax: tax,
    total: total
  }
}
```

### Type Guards

```httpdsl
fn is_string(value) {
  return type(value) == "string"
}

fn is_number(value) {
  t = type(value)
  return t == "int" || t == "float"
}

fn is_array(value) {
  return type(value) == "array"
}

route POST "/validate" json {
  data = request.data
  
  if !is_string(data.name) {
    response.status = 400
    response.body = {error: "name must be a string"}
    return
  }
  
  if !is_number(data.age) {
    response.status = 400
    response.body = {error: "age must be a number"}
    return
  }
  
  response.body = {valid: true}
}
```
