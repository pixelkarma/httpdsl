# Functional Builtins

- [map()](#map)
- [filter()](#filter)
- [reduce()](#reduce)
- [find()](#find)
- [some()](#some)
- [every()](#every)
- [count()](#count)
- [pluck()](#pluck)
- [group_by()](#group_by)
- [sum()](#sum)
- [min()](#min)
- [max()](#max)
- [Complete Examples](#complete-examples)
  - [Data Transformation](#data-transformation)
  - [Aggregation](#aggregation)
  - [Validation](#validation)
  - [Search](#search)
  - [Grouping](#grouping)

Higher-order functions for array operations.

## map()

Transform each element:

```httpdsl
map([1, 2, 3], fn(n) { return n * 2 })
map(["a", "b", "c"], fn(s) { return upper(s) })
```

## filter()

Select elements matching condition:

```httpdsl
filter([1, 2, 3, 4, 5], fn(n) { return n > 2 })
filter(["apple", "banana", "cherry"], fn(s) { return len(s) > 5 })
```

## reduce()

Reduce array to single value:

```httpdsl
reduce([1, 2, 3, 4], fn(acc, n) { return acc + n }, 0)
reduce(["a", "b", "c"], fn(acc, s) { return acc + s }, "")
```

## find()

Find first matching element:

```httpdsl
find([1, 2, 3, 4], fn(n) { return n > 2 })
find([{id: 1}, {id: 2}], fn(obj) { return obj.id == 2 })
```

## some()

Check if any element matches:

```httpdsl
some([1, 2, 3], fn(n) { return n > 2 })
some(["a", "b", "c"], fn(s) { return s == "x" })
```

## every()

Check if all elements match:

```httpdsl
every([2, 4, 6], fn(n) { return n % 2 == 0 })
every(["a", "b", "c"], fn(s) { return len(s) == 1 })
```

## count()

Count matching elements:

```httpdsl
count([1, 2, 3, 4, 5], fn(n) { return n > 2 })
count(["a", "ab", "abc"], fn(s) { return len(s) > 1 })
```

## pluck()

Extract field from objects:

```httpdsl
users = [{name: "Alice", age: 30}, {name: "Bob", age: 25}]
pluck(users, "name")
pluck(users, "age")
```

## group_by()

Group objects by field:

```httpdsl
items = [
  {category: "fruit", name: "apple"},
  {category: "fruit", name: "banana"},
  {category: "vegetable", name: "carrot"}
]
group_by(items, "category")
```

## sum()

Sum array elements:

```httpdsl
sum([1, 2, 3, 4, 5])
sum([10, 20, 30])
```

With transform function:

```httpdsl
items = [{price: 10}, {price: 20}, {price: 30}]
sum(items, fn(item) { return item.price })
```

## min()

Find minimum:

```httpdsl
min([3, 1, 4, 1, 5])
min([10, 5, 8])
```

With transform:

```httpdsl
items = [{value: 10}, {value: 5}, {value: 8}]
min(items, fn(item) { return item.value })
```

## max()

Find maximum:

```httpdsl
max([3, 1, 4, 1, 5])
max([10, 5, 8])
```

With transform:

```httpdsl
items = [{value: 10}, {value: 5}, {value: 8}]
max(items, fn(item) { return item.value })
```

## Complete Examples

### Data Transformation

```httpdsl
route GET "/users" {
  users = [
    {id: 1, name: "Alice", age: 30, active: true},
    {id: 2, name: "Bob", age: 25, active: false},
    {id: 3, name: "Charlie", age: 35, active: true}
  ]
  
  active_users = filter(users, fn(u) { return u.active })
  
  names = pluck(active_users, "name")
  
  upper_names = map(names, fn(n) { return upper(n) })
  
  response.body = {users: upper_names}
}
```

### Aggregation

```httpdsl
route GET "/stats" {
  orders = [
    {id: 1, amount: 100, status: "paid"},
    {id: 2, amount: 50, status: "paid"},
    {id: 3, amount: 75, status: "pending"}
  ]
  
  paid_orders = filter(orders, fn(o) { return o.status == "paid" })
  
  total = sum(paid_orders, fn(o) { return o.amount })
  
  avg = total / len(paid_orders)
  
  response.body = {
    total: total,
    average: avg,
    count: len(paid_orders)
  }
}
```

### Validation

```httpdsl
route POST "/validate-items" json {
  items = request.data.items ?? []
  
  all_valid = every(items, fn(item) {
    return has(item, "name") && has(item, "price") && item.price > 0
  })
  
  if !all_valid {
    response.status = 400
    response.body = {error: "Invalid items"}
    return
  }
  
  response.body = {valid: true}
}
```

### Search

```httpdsl
route GET "/search" {
  query = lower(request.query.q ?? "")
  
  items = [
    {id: 1, name: "Apple", price: 1.5},
    {id: 2, name: "Banana", price: 0.8},
    {id: 3, name: "Cherry", price: 2.0}
  ]
  
  results = filter(items, fn(item) {
    return contains(lower(item.name), query)
  })
  
  response.body = {results: results, count: len(results)}
}
```

### Grouping

```httpdsl
route GET "/products/by-category" {
  products = [
    {name: "Apple", category: "fruit", price: 1.5},
    {name: "Carrot", category: "vegetable", price: 0.8},
    {name: "Banana", category: "fruit", price: 1.0}
  ]
  
  grouped = group_by(products, "category")
  
  response.body = grouped
}
```
