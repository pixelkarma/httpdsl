# String Builtins

Functions for string manipulation.

## len()

Get string length:

```httpdsl
len("hello")
len("")
len("Hello, World!")
```

## trim()

Remove leading and trailing whitespace:

```httpdsl
trim("  hello  ")
trim("\n\ttext\n")
trim("no-trim")
```

## upper()

Convert to uppercase:

```httpdsl
upper("hello")
upper("Hello World")
upper("abc123")
```

## lower()

Convert to lowercase:

```httpdsl
lower("HELLO")
lower("Hello World")
lower("ABC123")
```

## split()

Split string into array:

```httpdsl
split("a,b,c", ",")
split("hello world", " ")
split("one-two-three", "-")
```

## join()

Join array into string:

```httpdsl
join(["a", "b", "c"], ",")
join(["hello", "world"], " ")
join(["one", "two", "three"], "-")
```

## replace()

Replace occurrences:

```httpdsl
replace("hello world", "world", "there")
replace("foo foo foo", "foo", "bar")
replace("test", "x", "y")
```

## starts_with()

Check if string starts with prefix:

```httpdsl
starts_with("hello world", "hello")
starts_with("test", "t")
starts_with("abc", "xyz")
```

## ends_with()

Check if string ends with suffix:

```httpdsl
ends_with("hello world", "world")
ends_with("test.txt", ".txt")
ends_with("abc", "xyz")
```

## contains()

Check if string contains substring:

```httpdsl
contains("hello world", "world")
contains("test", "es")
contains("abc", "xyz")
```

## index_of()

Find position of substring (returns -1 if not found):

```httpdsl
index_of("hello world", "world")
index_of("test", "es")
index_of("abc", "xyz")
```

## repeat()

Repeat string N times:

```httpdsl
repeat("ab", 3)
repeat("-", 10)
repeat("x", 0)
```

## slice()

Extract substring:

```httpdsl
slice("hello world", 0, 5)
slice("abcdef", 2, 5)
slice("test", 1, 3)
```

## pad_left()

Pad string on the left:

```httpdsl
pad_left("42", 5, "0")
pad_left("test", 10, " ")
pad_left("x", 3, "-")
```

## pad_right()

Pad string on the right:

```httpdsl
pad_right("hello", 10, " ")
pad_right("test", 8, ".")
pad_right("x", 5, "-")
```

## truncate()

Truncate string to max length:

```httpdsl
truncate("hello world", 5)
truncate("test", 10)
truncate("long text here", 8)
```

With ellipsis:

```httpdsl
text = "This is a very long text"
if len(text) > 20 {
  text = truncate(text, 17) + "..."
}
```

## capitalize()

Capitalize first letter:

```httpdsl
capitalize("hello")
capitalize("hello world")
capitalize("TEST")
```

## Complete Examples

### URL Parser

```httpdsl
route GET "/parse-url" {
  url = request.query.url ?? ""
  
  if !starts_with(url, "http://") && !starts_with(url, "https://") {
    response.status = 400
    response.body = {error: "Invalid URL"}
    return
  }
  
  protocol = starts_with(url, "https://") ? "https" : "http"
  
  without_protocol = replace(url, `${protocol}://`, "")
  parts = split(without_protocol, "/")
  domain = parts[0]
  
  response.body = {
    url: url,
    protocol: protocol,
    domain: domain,
    parts: parts
  }
}
```

### Text Formatting

```httpdsl
route POST "/format-text" json {
  text = request.data.text ?? ""
  format = request.data.format ?? "none"
  
  result = text
  
  switch format {
    case "upper" {
      result = upper(text)
    }
    case "lower" {
      result = lower(text)
    }
    case "title" {
      words = split(text, " ")
      titled = []
      
      each word in words {
        titled = append(titled, capitalize(lower(word)))
      }
      
      result = join(titled, " ")
    }
    case "trim" {
      result = trim(text)
    }
  }
  
  response.body = {result: result}
}
```

### String Validation

```httpdsl
route POST "/validate" json {
  username = request.data.username ?? ""
  
  errors = []
  
  if len(username) < 3 {
    errors = append(errors, "Username too short")
  }
  
  if len(username) > 20 {
    errors = append(errors, "Username too long")
  }
  
  if contains(username, " ") {
    errors = append(errors, "Username cannot contain spaces")
  }
  
  if !starts_with(username, "user_") {
    errors = append(errors, "Username must start with user_")
  }
  
  if len(errors) > 0 {
    response.status = 400
    response.body = {errors: errors}
  } else {
    response.body = {valid: true}
  }
}
```

### Search Filter

```httpdsl
route GET "/search" {
  query = lower(trim(request.query.q ?? ""))
  
  items = [
    {id: 1, name: "Apple"},
    {id: 2, name: "Banana"},
    {id: 3, name: "Cherry"},
    {id: 4, name: "Date"}
  ]
  
  results = []
  
  each item in items {
    if contains(lower(item.name), query) {
      results = append(results, item)
    }
  }
  
  response.body = {query: query, results: results}
}
```

### Slug Generator

```httpdsl
fn create_slug(text) {
  slug = lower(trim(text))
  slug = replace(slug, " ", "-")
  slug = replace(slug, "_", "-")
  return slug
}

route POST "/slugify" json {
  title = request.data.title ?? ""
  slug = create_slug(title)
  
  response.body = {
    title: title,
    slug: slug
  }
}
```

### CSV Parser

```httpdsl
route POST "/parse-csv" text {
  csv_data = request.data
  
  lines = split(csv_data, "\n")
  rows = []
  
  each line in lines {
    trimmed = trim(line)
    
    if trimmed != "" {
      columns = split(trimmed, ",")
      cleaned = []
      
      each col in columns {
        cleaned = append(cleaned, trim(col))
      }
      
      rows = append(rows, cleaned)
    }
  }
  
  response.body = {rows: rows, count: len(rows)}
}
```
