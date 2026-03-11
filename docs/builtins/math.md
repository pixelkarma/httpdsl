# Math Builtins

- [abs()](#abs)
- [ceil()](#ceil)
- [floor()](#floor)
- [round()](#round)
- [clamp()](#clamp)
- [rand()](#rand)
- [range()](#range)
- [Complete Examples](#complete-examples)
  - [Random Number Generator](#random-number-generator)
  - [Pagination with Range](#pagination-with-range)
  - [Clamping Input](#clamping-input)
  - [Rounding Prices](#rounding-prices)
  - [Progress Percentage](#progress-percentage)
  - [Random String Generator](#random-string-generator)
  - [Pagination Offset](#pagination-offset)
  - [Statistics](#statistics)
  - [Dice Roller](#dice-roller)
  - [Distance Calculation](#distance-calculation)

Mathematical functions and utilities.

## abs()

Absolute value:

```httpdsl
abs(-5)
abs(10)
abs(-3.14)
```

## ceil()

Round up to nearest integer:

```httpdsl
ceil(3.14)
ceil(3.01)
ceil(3.99)
ceil(-2.5)
```

## floor()

Round down to nearest integer:

```httpdsl
floor(3.14)
floor(3.99)
floor(-2.5)
```

## round()

Round to nearest integer:

```httpdsl
round(3.14)
round(3.5)
round(3.9)
round(-2.5)
```

## clamp()

Constrain value between min and max:

```httpdsl
clamp(5, 0, 10)
clamp(15, 0, 10)
clamp(-5, 0, 10)
```

## rand()

Generate random numbers:

```httpdsl
rand()
rand(10)
rand(5, 15)
```

- `rand()` - Random float between 0.0 and 1.0
- `rand(max)` - Random integer from 0 to max-1
- `rand(min, max)` - Random integer from min to max-1

## range()

Generate array of integers:

```httpdsl
range(5)
range(1, 10)
range(0, 100, 10)
```

- `range(n)` - 0 to n-1
- `range(start, end)` - start to end-1
- `range(start, end, step)` - start to end-1 with step

## Complete Examples

### Random Number Generator

```httpdsl
route GET "/random" {
  min = int(request.query.min ?? "1")
  max = int(request.query.max ?? "100")
  
  random_value = rand(min, max)
  
  response.body = {
    min: min,
    max: max,
    value: random_value
  }
}
```

### Pagination with Range

```httpdsl
route GET "/pages" {
  total_pages = 10
  pages = range(1, total_pages + 1)
  
  response.body = {pages: pages}
}
```

### Clamping Input

```httpdsl
route GET "/items" {
  page = int(request.query.page ?? "1")
  per_page = int(request.query.per_page ?? "10")
  
  page = clamp(page, 1, 100)
  per_page = clamp(per_page, 1, 100)
  
  response.body = {
    page: page,
    per_page: per_page
  }
}
```

### Rounding Prices

```httpdsl
route POST "/calculate-total" json {
  items = request.data.items ?? []
  
  total = 0.0
  
  each item in items {
    price = float(item.price ?? 0)
    quantity = int(item.quantity ?? 1)
    total += price * float(quantity)
  }
  
  rounded = round(total * 100.0) / 100.0
  
  response.body = {
    total: rounded,
    raw_total: total
  }
}
```

### Progress Percentage

```httpdsl
route GET "/progress/:current/:total" {
  current = int(request.params.current)
  total = int(request.params.total)
  
  percentage = 0
  
  if total > 0 {
    percentage = round((float(current) / float(total)) * 100.0)
  }
  
  percentage = clamp(percentage, 0, 100)
  
  response.body = {
    current: current,
    total: total,
    percentage: percentage
  }
}
```

### Random String Generator

```httpdsl
route GET "/random-code" {
  length = int(request.query.length ?? "6")
  length = clamp(length, 4, 20)
  
  chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
  code = ""
  
  each i in range(length) {
    index = rand(len(chars))
    code = code + slice(chars, index, index + 1)
  }
  
  response.body = {code: code}
}
```

### Pagination Offset

```httpdsl
route GET "/api/items" {
  page = int(request.query.page ?? "1")
  per_page = int(request.query.per_page ?? "10")
  
  page = clamp(page, 1, 1000)
  per_page = clamp(per_page, 1, 100)
  
  offset = (page - 1) * per_page
  
  items = range(offset, offset + per_page)
  
  response.body = {
    page: page,
    per_page: per_page,
    offset: offset,
    items: items
  }
}
```

### Statistics

```httpdsl
route POST "/stats" json {
  numbers = request.data.numbers ?? []
  
  if len(numbers) == 0 {
    response.status = 400
    response.body = {error: "No numbers provided"}
    return
  }
  
  total = sum(numbers)
  avg = float(total) / float(len(numbers))
  min_val = min(numbers)
  max_val = max(numbers)
  
  response.body = {
    count: len(numbers),
    sum: total,
    average: round(avg * 100.0) / 100.0,
    min: min_val,
    max: max_val,
    range: abs(max_val - min_val)
  }
}
```

### Dice Roller

```httpdsl
route GET "/roll/:sides" {
  sides = int(request.params.sides)
  count = int(request.query.count ?? "1")
  
  sides = clamp(sides, 2, 100)
  count = clamp(count, 1, 10)
  
  rolls = []
  total = 0
  
  each i in range(count) {
    roll = rand(1, sides + 1)
    rolls = append(rolls, roll)
    total += roll
  }
  
  response.body = {
    sides: sides,
    count: count,
    rolls: rolls,
    total: total
  }
}
```

### Distance Calculation

```httpdsl
route GET "/distance" {
  x1 = float(request.query.x1 ?? "0")
  y1 = float(request.query.y1 ?? "0")
  x2 = float(request.query.x2 ?? "0")
  y2 = float(request.query.y2 ?? "0")
  
  dx = abs(x2 - x1)
  dy = abs(y2 - y1)
  
  response.body = {
    point1: {x: x1, y: y1},
    point2: {x: x2, y: y2},
    dx: dx,
    dy: dy
  }
}
```
