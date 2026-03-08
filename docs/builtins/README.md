# HTTPDSL Builtin Functions Reference

Comprehensive documentation for all builtin functions available in HTTPDSL.

## Categories

### [String Functions](./strings.md)
String manipulation and text processing functions.
- `len`, `trim`, `upper`, `lower`, `split`, `join`, `replace`
- `starts_with`, `ends_with`, `contains`, `index_of`
- `repeat`, `slice`, `pad_left`, `pad_right`, `truncate`, `capitalize`

### [Array Functions](./arrays.md)
Functions for working with arrays (lists).
- `len`, `append`, `push`, `slice`, `reverse`, `unique`, `flat`, `chunk`
- `sort`, `sort_by`, `contains`, `index_of`

### [Hash Functions](./hashes.md)
Functions for manipulating hash maps (objects/dictionaries).
- `keys`, `values`, `merge`, `delete`, `contains`

### [Functional Iteration](./functional.md)
Higher-order functions for functional programming patterns.
- `map`, `filter`, `reduce`, `find`, `some`, `every`, `count`
- `pluck`, `group_by`, `sum`, `min`, `max`

### [Type Functions](./types.md)
Type checking and type conversion functions.
- `type`, `str`, `int`, `float`, `bool`

### [Math Functions](./math.md)
Mathematical and numeric utility functions.
- `abs`, `ceil`, `floor`, `round`, `clamp`
- `rand()`, `rand(max)`, `rand(min, max)`
- `range(n)`, `range(start, end)`, `range(start, end, step)`

### [Encoding Functions](./encoding.md)
Functions for encoding, decoding, and serializing data.
- `base64_encode`, `base64_decode`
- `url_encode`, `url_decode`
- `json.parse`, `json.stringify`

## Documentation Format

Each function includes:
- **Signature**: Type signature showing parameters and return type
- **Description**: What the function does
- **Return Value**: What the function returns
- **Examples**: Practical code examples
- **Edge Cases**: Special behaviors and gotchas

## Quick Reference

### Most Commonly Used

```httpdsl
// String operations
upper("hello")                    // "HELLO"
split("a,b,c", ",")               // ["a", "b", "c"]
join(["a", "b"], "-")             // "a-b"

// Array operations
len([1, 2, 3])                    // 3
map([1, 2, 3], fn(n) { n * 2 })   // [2, 4, 6]
filter([1, 2, 3], fn(n) { n > 1 }) // [2, 3]

// Hash operations
keys({a: 1, b: 2})                // ["a", "b"]
merge({a: 1}, {b: 2})             // {a: 1, b: 2}

// Type conversion
str(42)                           // "42"
int("42")                         // 42
type("hello")                     // "string"

// Math
abs(-5)                           // 5
round(3.7)                        // 4
rand(10)                          // 0-9

// Encoding
json.stringify({name: "Alice"})   // '{"name":"Alice"}'
json.parse('{"age": 30}')         // {age: 30}
base64_encode("hello")            // "aGVsbG8="
url_encode("hello world")         // "hello+world"
```

## Cross-Category Functions

Some functions work on multiple types:
- `len()` - works on strings and arrays
- `slice()` - works on strings and arrays
- `contains()` - works on strings and arrays
- `index_of()` - works on strings and arrays

## Notes

- All string and array functions return new values (immutable)
- Hash operations return new hashes (immutable)
- Functions are first-class values (can be passed as arguments)
- Type coercion happens automatically in many cases
