# Types

HTTPDSL is dynamically typed with automatic type coercion in certain contexts.

## Strings

Double-quoted strings:

```httpdsl
name = "Alice"
greeting = "Hello, World!"
empty = ""
```

Escape sequences:

```httpdsl
message = "Line 1\nLine 2\tTabbed"
path = "C:\\Users\\Alice"
quote = "She said \"Hello\""
```

## Template Strings

Backtick strings with interpolation:

```httpdsl
name = "Alice"
age = 30
message = `Hello, ${name}! You are ${age} years old.`

route GET "/greet/:name" {
  response.body = `Welcome, ${request.params.name}!`
}
```

Expressions in interpolations:

```httpdsl
x = 10
y = 20
result = `${x} + ${y} = ${x + y}`
```

## Integers

Decimal, hexadecimal, octal, and binary literals:

```httpdsl
dec = 42
oct = 0o77
negative = -100
```

## Floats

Floating-point numbers:

```httpdsl
pi = 3.14159
negative = -2.5
```

## Booleans

True and false:

```httpdsl
is_active = true
is_deleted = false
```

## Null

Represents absence of value:

```httpdsl
user = null

if user == null {
  response.body = "No user found"
}
```

## Arrays

Ordered collections:

```httpdsl
numbers = [1, 2, 3, 4, 5]
mixed = ["hello", 42, true, null]
empty = []
nested = [[1, 2], [3, 4]]
```

Access elements by index:

```httpdsl
fruits = ["apple", "banana", "cherry"]
first = fruits[0]
second = fruits[1]
```

Modify elements:

```httpdsl
numbers = [1, 2, 3]
numbers[0] = 10
```

## Hashes

Key-value pairs (objects/dictionaries):

```httpdsl
user = {
  name: "Alice",
  age: 30,
  email: "alice@example.com"
}
```

Quoted keys for special characters:

```httpdsl
config = {
  "api-key": "secret",
  "max-connections": 100
}
```

Nested hashes:

```httpdsl
user = {
  name: "Alice",
  address: {
    street: "123 Main St",
    city: "Boston"
  }
}
```

Access values:

```httpdsl
user = {name: "Alice", age: 30}
name = user.name
age = user["age"]
```

Modify values:

```httpdsl
user = {name: "Alice"}
user.age = 30
user["email"] = "alice@example.com"
```

## Type Coercion

### String Concatenation

Adding a string to any type converts the other type to string:

```httpdsl
result = "Count: " + 42
result = 100 + " items"
result = "Active: " + true
```

### Arithmetic

Numeric operations require numbers:

```httpdsl
sum = 10 + 20
product = 3.14 * 2
```

## Type Checking

Use the `type()` builtin:

```httpdsl
type("hello")
type(42)
type(3.14)
type(true)
type(null)
type([1, 2, 3])
type({name: "Alice"})
```

Example usage:

```httpdsl
route POST "/api/data" json {
  value = request.data.value
  
  if type(value) == "string" {
    response.body = {result: upper(value)}
  } else if type(value) == "int" {
    response.body = {result: value * 2}
  } else {
    response.status = 400
    response.body = {error: "Invalid type"}
  }
}
```

## Type Conversion

Explicit conversion builtins:

```httpdsl
str(42)
int("123")
int(3.14)
float("3.14")
float(42)
bool(1)
bool(0)
bool("")
```

Examples:

```httpdsl
route GET "/calculate" {
  a = int(request.query.a ?? "0")
  b = int(request.query.b ?? "0")
  
  response.body = {
    sum: a + b,
    product: a * b
  }
}
```

## Truthiness

In boolean contexts:

- **Falsy**: `false`, `null`, `0`, `0.0`, `""`
- **Truthy**: Everything else

```httpdsl
if "" {
  response.body = "Not executed"
}

if "hello" {
  response.body = "Executed"
}

if 0 {
  response.body = "Not executed"
}

if null {
  response.body = "Not executed"
}
```

## Working with JSON

Parse JSON strings:

```httpdsl
json_string = `{"name":"Alice","age":30}`
data = json.parse(json_string)
name = data.name
```

Serialize to JSON:

```httpdsl
data = {name: "Alice", age: 30}
json_string = json.stringify(data)
```

Route responses automatically serialize hashes and arrays to JSON:

```httpdsl
route GET "/api/user" {
  response.body = {name: "Alice", age: 30}
}
```
