# Functions

HTTPDSL supports named and anonymous functions with closures and multiple return values.

## Named Functions

Define functions with the `fn` keyword:

```httpdsl
fn greet(name) {
  return `Hello, ${name}!`
}

message = greet("Alice")
```

Multiple parameters:

```httpdsl
fn add(a, b) {
  return a + b
}

sum = add(10, 20)
```

## Anonymous Functions

Assign functions to variables:

```httpdsl
add = fn(a, b) {
  return a + b
}

result = add(5, 3)
```

Pass as arguments:

```httpdsl
numbers = [1, 2, 3, 4, 5]

doubled = map(numbers, fn(n) {
  return n * 2
})
```

## Return Statement

Return a value from a function:

```httpdsl
fn square(n) {
  return n * n
}
```

Early return:

```httpdsl
fn divide(a, b) {
  if b == 0 {
    return null
  }
  return a / b
}
```

## Multiple Return Values

Return multiple values:

```httpdsl
fn get_user(id) {
  if id == 1 {
    return {id: 1, name: "Alice"}, null
  }
  return null, "User not found"
}

user, err = get_user(1)

if err != null {
  log_error(err)
} else {
  log_info(user.name)
}
```

In route handlers:

```httpdsl
fn validate_email(email) {
  if email == "" {
    return false, "Email is required"
  }
  if !is_email(email) {
    return false, "Invalid email format"
  }
  return true, null
}

route POST "/api/register" json {
  email = request.data.email ?? ""
  
  valid, err = validate_email(email)
  
  if !valid {
    response.status = 400
    response.body = {error: err}
    return
  }
  
  response.body = {message: "Registration successful"}
}
```

## Closures

Functions capture variables from their enclosing scope by reference:

```httpdsl
fn make_counter() {
  count = 0
  
  return fn() {
    count += 1
    return count
  }
}

counter1 = make_counter()
counter2 = make_counter()

log(counter1())
log(counter1())
log(counter2())
```

Capturing route context:

```httpdsl
api_key = env("API_KEY")

fn check_auth() {
  token = request.headers["authorization"] ?? ""
  return token == `Bearer ${api_key}`
}

route GET "/api/protected" {
  if !check_auth() {
    response.status = 401
    response.body = {error: "Unauthorized"}
    return
  }
  
  response.body = {data: "Protected data"}
}
```

## Higher-Order Functions

Functions that take or return functions:

```httpdsl
fn apply_twice(f, x) {
  return f(f(x))
}

fn double(n) {
  return n * 2
}

result = apply_twice(double, 5)
```

Factory functions:

```httpdsl
fn make_multiplier(factor) {
  return fn(n) {
    return n * factor
  }
}

double = make_multiplier(2)
triple = make_multiplier(3)

log(double(5))
log(triple(5))
```

## Recursion

Functions can call themselves:

```httpdsl
fn factorial(n) {
  if n <= 1 {
    return 1
  }
  return n * factorial(n - 1)
}

result = factorial(5)
```

Fibonacci:

```httpdsl
fn fibonacci(n) {
  if n <= 1 {
    return n
  }
  return fibonacci(n - 1) + fibonacci(n - 2)
}
```

## Practical Examples

### Password Validation

```httpdsl
fn validate_password(password) {
  if password == "" {
    return false, "Password is required"
  }
  
  if len(password) < 8 {
    return false, "Password must be at least 8 characters"
  }
  
  if !contains(password, "0123456789") {
    return false, "Password must contain at least one number"
  }
  
  return true, null
}

route POST "/api/register" json {
  password = request.data.password ?? ""
  
  valid, err = validate_password(password)
  
  if !valid {
    response.status = 400
    response.body = {error: err}
    return
  }
  
  hashed = hash_password(password)
  response.body = {message: "Password accepted"}
}
```

### Database Query Helper

```httpdsl
db_conn = null

fn get_db() {
  if db_conn == null {
    db_conn = db.open("sqlite", "./data.db")
  }
  return db_conn
}

fn find_user_by_id(id) {
  conn = get_db()
  user = conn.query_one("SELECT * FROM users WHERE id = ?", [id])
  
  if user == null {
    return null, "User not found"
  }
  
  return user, null
}

route GET "/api/users/:id" {
  user_id = int(request.params.id)
  
  user, err = find_user_by_id(user_id)
  
  if err != null {
    response.status = 404
    response.body = {error: err}
  } else {
    response.body = user
  }
}
```

### Pagination Helper

```httpdsl
fn paginate(items, page, per_page) {
  total = len(items)
  total_pages = ceil(float(total) / float(per_page))
  
  if page < 1 {
    page = 1
  }
  
  if page > total_pages {
    page = total_pages
  }
  
  start = (page - 1) * per_page
  end = start + per_page
  
  if end > total {
    end = total
  }
  
  return {
    items: slice(items, start, end),
    page: page,
    per_page: per_page,
    total: total,
    total_pages: total_pages
  }
}

route GET "/api/items" {
  all_items = range(100)
  
  page = int(request.query.page ?? "1")
  per_page = int(request.query.per_page ?? "10")
  
  result = paginate(all_items, page, per_page)
  response.body = result
}
```

### Response Helpers

```httpdsl
fn success(data) {
  response.status = 200
  response.body = {success: true, data: data}
}

fn error(code, message) {
  response.status = code
  response.body = {success: false, error: message}
}

fn created(data) {
  response.status = 201
  response.body = {success: true, data: data}
}

route POST "/api/users" json {
  {name, email} = request.data
  
  if name == "" || email == "" {
    error(400, "Missing required fields")
    return
  }
  
  user = {
    id: cuid2(),
    name: name,
    email: email,
    created_at: now()
  }
  
  created(user)
}

route GET "/api/users/:id" {
  user_id = request.params.id
  
  user = {id: user_id, name: "Sample User"}
  
  if user == null {
    error(404, "User not found")
  } else {
    success(user)
  }
}
```

### Middleware Functions

```httpdsl
fn require_auth() {
  token = request.bearer
  
  if token == "" {
    response.status = 401
    response.body = {error: "Missing authentication token"}
    return false
  }
  
  return true
}

fn require_role(required_role) {
  user_role = request.session.role ?? "guest"
  
  if user_role != required_role {
    response.status = 403
    response.body = {error: "Insufficient permissions"}
    return false
  }
  
  return true
}

route DELETE "/api/users/:id" {
  if !require_auth() {
    return
  }
  
  if !require_role("admin") {
    return
  }
  
  user_id = request.params.id
  response.body = {deleted: user_id}
}
```
