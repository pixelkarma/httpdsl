# Validation Builtins

Functions for data validation.

## validate()

Validate a data object against a schema of pipe-delimited rule strings:

```httpdsl
errors = validate({name: "Alice", email: "alice@example.com"}, {
    name: "required|string|min:3|max:50",
    email: "required|email"
})
// null (all valid)
```

Returns `null` if all rules pass. Returns a map of `{fieldName: "failedRule"}` if any rule fails (only the first failure per field is reported):

```httpdsl
errors = validate({name: "", email: "bad"}, {
    name: "required|string|min:3",
    email: "required|email"
})
// {name: "required", email: "email"}
```

Both arguments must be objects. The first is the data to validate, the second is the schema where each key is a field name and each value is a string of rules separated by `|`.

### Available Rules

#### Type Rules

`required` — field must exist and not be `null` or empty string `""`:

```httpdsl
validate({}, {name: "required"})
// {name: "required"}

validate({name: ""}, {name: "required"})
// {name: "required"}

validate({name: "Alice"}, {name: "required"})
// null
```

`string` — value must be a string (skipped if field is missing or null):

```httpdsl
validate({name: 42}, {name: "string"})
// {name: "string"}

validate({name: "Alice"}, {name: "string"})
// null
```

`int` — value must be an integer. A float that is a whole number (e.g. `3.0`) passes:

```httpdsl
validate({age: "twenty"}, {age: "int"})
// {age: "int"}

validate({age: 25}, {age: "int"})
// null
```

`number` — value must be numeric (int or float):

```httpdsl
validate({score: "high"}, {score: "number"})
// {score: "number"}

validate({score: 3.14}, {score: "number"})
// null
```

`bool` — value must be a boolean:

```httpdsl
validate({active: "yes"}, {active: "bool"})
// {active: "bool"}

validate({active: true}, {active: "bool"})
// null
```

`array` — value must be an array:

```httpdsl
validate({tags: "one"}, {tags: "array"})
// {tags: "array"}

validate({tags: ["one", "two"]}, {tags: "array"})
// null
```

`object` — value must be an object/map:

```httpdsl
validate({meta: "text"}, {meta: "object"})
// {meta: "object"}

validate({meta: {key: "val"}}, {meta: "object"})
// null
```

#### Format Rules

`email` — value must match an email pattern:

```httpdsl
validate({email: "bad"}, {email: "email"})
// {email: "email"}

validate({email: "alice@example.com"}, {email: "email"})
// null
```

`url` — value must start with `http://` or `https://`:

```httpdsl
validate({site: "example.com"}, {site: "url"})
// {site: "url"}

validate({site: "https://example.com"}, {site: "url"})
// null
```

`uuid` — value must match UUID format:

```httpdsl
validate({id: "abc"}, {id: "uuid"})
// {id: "uuid"}

validate({id: "550e8400-e29b-41d4-a716-446655440000"}, {id: "uuid"})
// null
```

`regex:pattern` — value must match the given regex pattern:

```httpdsl
validate({code: "abc"}, {code: "regex:^[A-Z0-9]{4}$"})
// {code: "regex:^[A-Z0-9]{4}$"}

validate({code: "AB12"}, {code: "regex:^[A-Z0-9]{4}$"})
// null
```

#### Size Rules

`min:N` — for strings and arrays, length must be >= N. For numbers, value must be >= N:

```httpdsl
validate({name: "Al"}, {name: "min:3"})
// {name: "min:3"}

validate({name: "Alice"}, {name: "min:3"})
// null

validate({age: 15}, {age: "min:18"})
// {age: "min:18"}
```

`max:N` — for strings and arrays, length must be <= N. For numbers, value must be <= N:

```httpdsl
validate({name: "A very long name here"}, {name: "max:10"})
// {name: "max:10"}

validate({name: "Alice"}, {name: "max:10"})
// null

validate({age: 200}, {age: "max:120"})
// {age: "max:120"}
```

`between:lo,hi` — for strings and arrays, length must be between lo and hi inclusive. For numbers, value must be between lo and hi inclusive:

```httpdsl
validate({age: 15}, {age: "between:18,120"})
// {age: "between:18,120"}

validate({age: 25}, {age: "between:18,120"})
// null
```

#### Choice Rules

`in:a,b,c` — value (converted to string) must be one of the comma-separated options:

```httpdsl
validate({role: "superadmin"}, {role: "in:admin,user,guest"})
// {role: "in:admin,user,guest"}

validate({role: "admin"}, {role: "in:admin,user,guest"})
// null
```

### Rule Behavior Notes

All rules except `required` are skipped when the field is missing or `null`. This means optional fields only get validated when present:

```httpdsl
// "role" is optional but must be valid if provided
validate({name: "Alice"}, {role: "in:admin,user,guest"})
// null (role is missing, so "in" rule is skipped)

validate({name: "Alice", role: "bad"}, {role: "in:admin,user,guest"})
// {role: "in:admin,user,guest"}
```

Combine `required` with other rules when the field must be present:

```httpdsl
validate({}, {email: "required|email"})
// {email: "required"} (fails on "required" before reaching "email")
```

Only the first failing rule per field is reported:

```httpdsl
validate({name: ""}, {name: "required|string|min:3"})
// {name: "required"} (stops at first failure)
```

## is_email()

Check if a string is a valid email address:

```httpdsl
is_email("alice@example.com")   // true
is_email("invalid-email")       // false
is_email("")                    // false
```

## is_url()

Check if a string is a URL (starts with `http://` or `https://`):

```httpdsl
is_url("https://example.com")   // true
is_url("http://localhost:3000") // true
is_url("ftp://files.example")   // false
is_url("not-a-url")             // false
```

## is_uuid()

Check if a string is a valid UUID:

```httpdsl
is_uuid("550e8400-e29b-41d4-a716-446655440000")  // true
is_uuid("invalid-uuid")                          // false
is_uuid("")                                      // false
```

## is_numeric()

Check if a value can be parsed as a number (integer or float):

```httpdsl
is_numeric("12345")    // true
is_numeric("3.14")     // true
is_numeric("abc")      // false
is_numeric("")         // false
```

## Complete Examples

### User Registration

```httpdsl
route POST "/auth/register" json {
    errors = validate(request.data, {
        username: "required|string|min:3|max:20",
        email: "required|email",
        password: "required|string|min:8",
        age: "required|int|min:18"
    })

    if errors != null {
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
    errors = validate(request.data, {
        name: "required|string|min:1|max:100",
        price: "required|number|min:0",
        category: "required|in:electronics,clothing,food",
        sku: "required|regex:^[A-Z0-9]{8}$"
    })

    if errors != null {
        response.status = 400
        response.body = {errors: errors}
        return
    }

    response.status = 201
    response.body = {
        id: cuid2(),
        name: request.data.name,
        price: request.data.price,
        category: request.data.category,
        sku: request.data.sku
    }
}
```

### Settings Update with Optional Fields

```httpdsl
route PUT "/settings" json {
    errors = validate(request.data, {
        theme: "in:light,dark,auto",
        language: "in:en,es,fr,de",
        notifications: "bool",
        max_items: "int|between:1,100"
    })

    if errors != null {
        response.status = 400
        response.body = {errors: errors}
        return
    }

    response.body = {updated: true, settings: request.data}
}
```

### Standalone Checks in Routes

```httpdsl
route POST "/shorten-url" json {
    url = request.data.url ?? ""

    if !is_url(url) {
        response.status = 400
        response.body = {error: "Invalid URL"}
        return
    }

    short_id = cuid2(8)
    store.set("url:" + short_id, url)

    response.body = {
        original: url,
        short: "/s/" + short_id
    }
}
```

### Combining validate() with Standalone Checks

```httpdsl
route POST "/api/contact" json {
    errors = validate(request.data, {
        name: "required|string|min:2|max:100",
        email: "required|email",
        message: "required|string|min:10|max:1000",
        priority: "in:low,normal,high"
    })

    if errors != null {
        response.status = 400
        response.body = {errors: errors}
        return
    }

    response.body = {message: "Contact form submitted"}
}
```

### Validating Items in a Loop

```httpdsl
route POST "/api/bulk-create" json {
    items = request.data.items ?? []
    all_errors = {}

    i = 0
    each item in items {
        errors = validate(item, {
            name: "required|string|min:1",
            price: "required|number|min:0"
        })

        if errors != null {
            all_errors[str(i)] = errors
        }

        i = i + 1
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
