# Array Builtins

Functions for array manipulation.

## len()

Get array length:

```httpdsl
len([1, 2, 3])
len([])
len(["a", "b", "c", "d"])
```

## append()

Returns a **new** array with the element added. Does not modify the original:

```httpdsl
new_arr = append([1, 2, 3], 4)   // [1, 2, 3, 4]
arr = append(arr, "item")         // must reassign
```

## push()

Append an element to an array **in place** (mutates the array):

```httpdsl
items = []
push(items, "first")
push(items, "second")
// items is now ["first", "second"]
```

Use `push` when building arrays in loops:

```httpdsl
results = []
each item in source {
  if item.active {
    push(results, item.name)
  }
}
```

`push(arr, item)` is equivalent to `arr = append(arr, item)` but more concise.

## slice()

Extract subarray:

```httpdsl
slice([1, 2, 3, 4, 5], 0, 3)
slice(["a", "b", "c", "d"], 1, 3)
slice([10, 20, 30], 0, 2)
```

## reverse()

Reverse array:

```httpdsl
reverse([1, 2, 3])
reverse(["a", "b", "c"])
reverse([5])
```

## unique()

Remove duplicates:

```httpdsl
unique([1, 2, 2, 3, 3, 3])
unique(["a", "b", "a", "c"])
unique([1, 1, 1])
```

## flat()

Flatten nested arrays:

```httpdsl
flat([[1, 2], [3, 4], [5]])
flat([["a", "b"], ["c"]])
flat([[1], [2], [3]])
```

## chunk()

Split array into chunks:

```httpdsl
chunk([1, 2, 3, 4, 5], 2)
chunk(["a", "b", "c", "d", "e"], 3)
chunk([1, 2, 3], 5)
```

## sort()

Sort array:

```httpdsl
sort([3, 1, 2])
sort(["c", "a", "b"])
sort([10, 5, 8, 1])
```

## sort_by()

Sort array of objects by key:

```httpdsl
users = [
  {name: "Charlie", age: 30},
  {name: "Alice", age: 25},
  {name: "Bob", age: 35}
]

sort_by(users, "name")
sort_by(users, "age")
```

## contains() / has() / includes()

Check if array contains value:

```httpdsl
contains([1, 2, 3], 2)
has(["a", "b", "c"], "b")
includes([10, 20, 30], 40)
```

## index_of()

Find index of element (returns -1 if not found):

```httpdsl
index_of([1, 2, 3], 2)
index_of(["a", "b", "c"], "b")
index_of([10, 20], 30)
```

## Complete Examples

### Pagination

```httpdsl
route GET "/items" {
  all_items = range(100)
  
  page = int(request.query.page ?? "1")
  per_page = int(request.query.per_page ?? "10")
  
  start = (page - 1) * per_page
  end = start + per_page
  
  items = slice(all_items, start, end)
  
  response.body = {
    page: page,
    per_page: per_page,
    total: len(all_items),
    items: items
  }
}
```

### Deduplication

```httpdsl
route POST "/deduplicate" json {
  items = request.data.items ?? []
  
  deduplicated = unique(items)
  
  response.body = {
    original_count: len(items),
    unique_count: len(deduplicated),
    items: deduplicated
  }
}
```

### Sorting

```httpdsl
route GET "/users" {
  users = [
    {id: 1, name: "Charlie", score: 85},
    {id: 2, name: "Alice", score: 95},
    {id: 3, name: "Bob", score: 90}
  ]
  
  sort_by_param = request.query.sort ?? "name"
  
  sorted_users = sort_by(users, sort_by_param)
  
  if request.query.order == "desc" {
    sorted_users = reverse(sorted_users)
  }
  
  response.body = {users: sorted_users}
}
```

### Chunking for Batch Processing

```httpdsl
route POST "/process-batch" json {
  items = request.data.items ?? []
  batch_size = int(request.data.batch_size ?? "10")
  
  batches = chunk(items, batch_size)
  
  results = []
  
  each batch in batches {
    batch_result = {
      size: len(batch),
      items: batch
    }
    results = append(results, batch_result)
  }
  
  response.body = {
    total_items: len(items),
    batch_count: len(batches),
    batches: results
  }
}
```

### Array Filtering

```httpdsl
route GET "/filter" {
  numbers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
  
  min_val = int(request.query.min ?? "0")
  max_val = int(request.query.max ?? "100")
  
  filtered = []
  
  each num in numbers {
    if num >= min_val && num <= max_val {
      filtered = append(filtered, num)
    }
  }
  
  response.body = {
    original: numbers,
    filtered: filtered,
    count: len(filtered)
  }
}
```

### Nested Array Flattening

```httpdsl
route POST "/flatten" json {
  nested = request.data.array ?? []
  
  flattened = flat(nested)
  
  response.body = {
    original: nested,
    flattened: flattened,
    original_length: len(nested),
    flattened_length: len(flattened)
  }
}
```

### Array Reversal

```httpdsl
route POST "/reverse" json {
  items = request.data.items ?? []
  
  reversed = reverse(items)
  
  response.body = {
    original: items,
    reversed: reversed
  }
}
```

### Array Search

```httpdsl
route GET "/find" {
  items = ["apple", "banana", "cherry", "date", "elderberry"]
  
  search_term = request.query.term ?? ""
  
  if contains(items, search_term) {
    index = index_of(items, search_term)
    
    response.body = {
      found: true,
      term: search_term,
      index: index
    }
  } else {
    response.body = {
      found: false,
      term: search_term
    }
  }
}
```

### Building Arrays

```httpdsl
route GET "/range" {
  start = int(request.query.start ?? "1")
  end = int(request.query.end ?? "10")
  
  items = []
  
  i = start
  while i <= end {
    items = append(items, i)
    i += 1
  }
  
  response.body = {
    start: start,
    end: end,
    items: items,
    count: len(items)
  }
}
```

### Array Intersection

```httpdsl
route POST "/intersect" json {
  array1 = request.data.array1 ?? []
  array2 = request.data.array2 ?? []
  
  intersection = []
  
  each item in array1 {
    if contains(array2, item) && !contains(intersection, item) {
      intersection = append(intersection, item)
    }
  }
  
  response.body = {
    array1: array1,
    array2: array2,
    intersection: intersection
  }
}
```

### Array Difference

```httpdsl
route POST "/difference" json {
  array1 = request.data.array1 ?? []
  array2 = request.data.array2 ?? []
  
  difference = []
  
  each item in array1 {
    if !contains(array2, item) {
      difference = append(difference, item)
    }
  }
  
  response.body = {
    array1: array1,
    array2: array2,
    difference: difference
  }
}
```
