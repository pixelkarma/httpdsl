# Operators

HTTPDSL supports standard operators for arithmetic, comparison, logic, and more.

## Arithmetic Operators

```httpdsl
sum = 10 + 5
diff = 10 - 5
product = 10 * 5
quotient = 10 / 5
remainder = 10 % 3
```

### String Concatenation

The `+` operator concatenates when either operand is a string:

```httpdsl
message = "Count: " + 42
path = "/api/v" + 1
full_name = "Alice" + " " + "Smith"
```

## Comparison Operators

```httpdsl
equal = 10 == 10
not_equal = 10 != 5
less = 5 < 10
greater = 10 > 5
less_equal = 5 <= 10
greater_equal = 10 >= 5
```

### String Comparison

```httpdsl
name = "Alice"
is_alice = name == "Alice"
is_not_bob = name != "Bob"
```

### Null Comparison

```httpdsl
user = null
is_null = user == null
is_not_null = user != null
```

## Logical Operators

### AND (`&&`)

Returns true only if both operands are truthy:

```httpdsl
if age >= 18 && has_license {
  response.body = "Can drive"
}
```

### OR (`||`)

Returns true if either operand is truthy:

```httpdsl
if is_admin || is_moderator {
  response.body = "Has privileges"
}
```

### NOT (`!`)

Negates a boolean value:

```httpdsl
if !is_logged_in {
  redirect("/login")
}
```

## Ternary Operator

Conditional expression:

```httpdsl
status = age >= 18 ? "adult" : "minor"
message = count > 0 ? "Items found" : "No items"
```

In route handlers:

```httpdsl
route GET "/api/status" {
  is_production = env("ENV") == "production"
  
  response.body = {
    env: is_production ? "prod" : "dev",
    debug: is_production ? false : true
  }
}
```

## Nullish Coalescing (`??`)

Returns the right operand only if the left is `null` (not for other falsy values):

```httpdsl
name = user.name ?? "Guest"
limit = request.query.limit ?? "10"
default_role = config.role ?? "user"
```

### Difference from OR

```httpdsl
value = 0

with_or = value || 10

with_nullish = value ?? 10
```

`||` treats `0`, `""`, and `false` as falsy, while `??` only checks for `null`.

## Operator Precedence

From highest to lowest:

1. **Unary**: `!`, `-` (negation)
2. **Multiplicative**: `*`, `/`, `%`
3. **Additive**: `+`, `-`
4. **Comparison**: `<`, `>`, `<=`, `>=`
5. **Equality**: `==`, `!=`
6. **Logical AND**: `&&`
7. **Nullish coalescing**: `??`
8. **Logical OR**: `||`
9. **Ternary**: `? :`

### Grouping with Parentheses

```httpdsl
result = (10 + 5) * 2
average = (a + b + c) / 3
complex = (x > 0 && y > 0) || (x < 0 && y < 0)
```

## Practical Examples

### Authentication Check

```httpdsl
route GET "/api/protected" {
  token = request.bearer
  
  if token == "" || token == null {
    response.status = 401
    response.body = {error: "Unauthorized"}
    return
  }
  
  response.body = {data: "Secret data"}
}
```

### Pagination

```httpdsl
route GET "/api/items" {
  page = int(request.query.page ?? "1")
  per_page = int(request.query.per_page ?? "10")
  
  page = page < 1 ? 1 : page
  per_page = clamp(per_page, 1, 100)
  
  offset = (page - 1) * per_page
  
  response.body = {
    page: page,
    per_page: per_page,
    offset: offset
  }
}
```

### Role-Based Access

```httpdsl
route DELETE "/api/users/:id" {
  role = request.session.role ?? "guest"
  
  if role != "admin" && role != "moderator" {
    response.status = 403
    response.body = {error: "Forbidden"}
    return
  }
  
  user_id = request.params.id
  response.body = {deleted: user_id}
}
```

### Input Validation

```httpdsl
route POST "/api/register" json {
  {email, password, age} = request.data
  
  if email == "" || password == "" || age == null {
    response.status = 400
    response.body = {error: "Missing required fields"}
    return
  }
  
  if len(password) < 8 {
    response.status = 400
    response.body = {error: "Password too short"}
    return
  }
  
  if age < 18 {
    response.status = 400
    response.body = {error: "Must be 18 or older"}
    return
  }
  
  response.status = 201
  response.body = {message: "Registration successful"}
}
```

### Default Values

```httpdsl
route GET "/api/config" {
  theme = request.query.theme ?? "light"
  lang = request.query.lang ?? "en"
  notifications = request.query.notifications ?? "true"
  
  response.body = {
    theme: theme,
    language: lang,
    notifications: notifications == "true"
  }
}
```

### Complex Conditions

```httpdsl
route POST "/api/submit" json {
  user_role = request.session.role ?? "guest"
  is_owner = request.data.user_id == request.session.user_id
  is_admin = user_role == "admin"
  
  can_edit = is_owner || is_admin
  
  if !can_edit {
    response.status = 403
    response.body = {error: "Cannot edit this resource"}
    return
  }
  
  response.body = {success: true}
}
```
