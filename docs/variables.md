# Variables

- [Declaration and Assignment](#declaration-and-assignment)
- [Reassignment](#reassignment)
- [Compound Assignment](#compound-assignment)
- [Scope](#scope)
- [Array Destructuring](#array-destructuring)
- [Object Destructuring](#object-destructuring)
- [Reserved Words](#reserved-words)
- [Built-in Variables](#built-in-variables)
- [Builtin Name Collision](#builtin-name-collision)
- [Closures](#closures)
- [Global Variables](#global-variables)
- [Practical Examples](#practical-examples)
  - [Request Data Extraction](#request-data-extraction)
  - [Path and Query Parameters](#path-and-query-parameters)
  - [Multiple Return Values](#multiple-return-values)

HTTPDSL uses dynamic typing with function scope and closure capture.

## Declaration and Assignment

No keywords required:

```httpdsl
name = "Alice"
age = 30
is_active = true
```

## Reassignment

```httpdsl
count = 0
count = 1
count = count + 1
```

## Compound Assignment

```httpdsl
x = 10
x += 5
x -= 3
```

Supported operators: `+=`, `-=`

## Scope

Variables are function-scoped:

```httpdsl
x = 10

fn example() {
  y = 20
  return x + y
}

result = example()
```

Block scope:

```httpdsl
x = 10

if true {
  y = 20
  x = 30
}
```

## Array Destructuring

Unpack array elements:

```httpdsl
coords = [10, 20]
[x, y] = coords
```

With function returns:

```httpdsl
fn get_coords() {
  return 100, 200
}

[x, y] = get_coords()
```

Nested arrays:

```httpdsl
data = [[1, 2], [3, 4]]
[first, second] = data
[a, b] = first
```

## Object Destructuring

Extract hash fields:

```httpdsl
user = {name: "Alice", age: 30, email: "alice@example.com"}
{name, age} = user
```

In route handlers:

```httpdsl
route POST "/api/users" json {
  {name, email, age} = request.data
  
  response.body = {
    id: cuid2(),
    name: name,
    email: email,
    age: age
  }
}
```

Nested destructuring:

```httpdsl
user = {
  name: "Alice",
  address: {
    city: "Boston",
    state: "MA"
  }
}

{name, address} = user
{city, state} = address
```

## Reserved Words

Cannot be used as variable names:

- `route`, `fn`, `return`, `if`, `else`, `while`, `each`, `in`
- `server`, `json`, `text`, `true`, `false`, `null`
- `env`, `file`, `db`, `break`, `continue`
- `try`, `catch`, `throw`, `async`
- `group`, `jwt`, `switch`, `case`, `default`

## Built-in Variables

These identifiers have special meaning and should not be reassigned:

- `args` — read-only map of `--key value` CLI flags (see [Configuration](env.md))
- `request` — the current HTTP request (available in route handlers)
- `response` — the current HTTP response (available in route handlers)

## Builtin Name Collision

Builtin function names compile to `builtin_X` internally. Avoid using these as variable names:

- String functions: `len`, `trim`, `upper`, `lower`, `split`, `join`, `replace`, `starts_with`, `ends_with`, `contains`, `index_of`, `repeat`, `slice`, `pad_left`, `pad_right`, `truncate`, `capitalize`
- Array functions: `append`, `push`, `reverse`, `unique`, `flat`, `chunk`, `sort`, `sort_by`
- Hash functions: `keys`, `values`, `merge`, `delete`
- Functional: `map`, `filter`, `reduce`, `find`, `some`, `every`, `count`, `pluck`, `group_by`, `sum`, `min`, `max`
- Types: `type`, `str`, `int`, `float`, `bool`
- Math: `abs`, `ceil`, `floor`, `round`, `clamp`, `rand`, `range`
- Encoding: `base64_encode`, `base64_decode`, `url_encode`, `url_decode`
- Crypto: `hash`, `hmac`, `uuid`, `cuid2`
- Validation: `validate`, `is_email`, `is_url`, `is_uuid`, `is_numeric`
- DateTime: `date`, `date_format`, `date_parse`, `strtotime`
- Fetch/Exec: `fetch`, `exec`
- Logging: `print`, `log`, `log_info`, `log_warn`, `log_error`, `sleep`, `server_stats`
- Navigation: `redirect`

## Closures

Functions capture variables by reference:

```httpdsl
fn make_counter() {
  count = 0
  
  return fn() {
    count += 1
    return count
  }
}

counter = make_counter()
log(counter())
log(counter())
log(counter())
```

## Global Variables

Top-level variable assignments are not allowed.

To create globals, assign variables inside `init {}`. Variables set in `init` are accessible everywhere:

```httpdsl
init {
  api_version = "v1"
  max_items = 100
}

route GET "/api/info" {
  response.body = {
    version: api_version,
    max_items: max_items
  }
}

route GET "/api/items" {
  limit = int(request.query.limit ?? str(max_items))
  limit = clamp(limit, 1, max_items)
  
  response.body = {
    items: range(limit)
  }
}
```

See [Init](init.md) for details.

## Practical Examples

### Request Data Extraction

```httpdsl
route POST "/api/login" json {
  {email, password} = request.data
  
  if email == "" || password == "" {
    response.status = 400
    response.body = {error: "Missing credentials"}
    return
  }
  
  response.body = {token: "jwt-token-here"}
}
```

### Path and Query Parameters

```httpdsl
route GET "/users/:id" {
  user_id = int(request.params.id)
  include_posts = request.query.posts == "true"
  
  user = {id: user_id, name: "User " + str(user_id)}
  
  if include_posts {
    user.posts = [{id: 1, title: "Post 1"}]
  }
  
  response.body = user
}
```

### Multiple Return Values

```httpdsl
fn divide(a, b) {
  if b == 0 {
    return null, "Division by zero"
  }
  return a / b, null
}

route GET "/calculate" {
  a = int(request.query.a ?? "10")
  b = int(request.query.b ?? "2")
  
  result, err = divide(a, b)
  
  if err != null {
    response.status = 400
    response.body = {error: err}
  } else {
    response.body = {result: result}
  }
}
```
