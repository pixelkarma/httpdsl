# Databases

HTTPDSL supports SQLite, PostgreSQL, MySQL, and MongoDB.

## Opening Connections

### SQLite

```httpdsl
conn = db.open("sqlite", "./database.db")
conn = db.open("sqlite", ":memory:")
```

### PostgreSQL

```httpdsl
conn = db.open("postgres", "host=localhost port=5432 user=postgres password=secret dbname=mydb sslmode=disable")
```

### MySQL

```httpdsl
conn = db.open("mysql", "user:password@tcp(localhost:3306)/dbname")
```

### MongoDB

```httpdsl
conn = db.open("mongo", "mongodb://localhost:27017")
```

## SQL Operations

### exec()

Execute statements that don't return rows:

```httpdsl
conn = db.open("sqlite", "./app.db")

result = conn.exec(`
  CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    name TEXT,
    email TEXT UNIQUE
  )
`, [])

log_info(`Rows affected: ${result.rows_affected}`)
```

Insert with last ID:

```httpdsl
result = conn.exec(
  "INSERT INTO users (name, email) VALUES (?, ?)",
  ["Alice", "alice@example.com"]
)

log_info(`Inserted ID: ${result.last_insert_id}`)
```

### query()

Fetch multiple rows:

```httpdsl
users = conn.query("SELECT * FROM users", [])

each user in users {
  log_info(`${user.id}: ${user.name}`)
}
```

With parameters:

```httpdsl
users = conn.query(
  "SELECT * FROM users WHERE email LIKE ?",
  ["%@example.com"]
)
```

### query_one()

Fetch a single row:

```httpdsl
user = conn.query_one(
  "SELECT * FROM users WHERE id = ?",
  [1]
)

if user != null {
  log_info(user.name)
} else {
  log_info("User not found")
}
```

### query_value()

Fetch a single scalar value:

```httpdsl
count = conn.query_value("SELECT COUNT(*) FROM users", [])
max_id = conn.query_value("SELECT MAX(id) FROM users", [])
```

### close()

Close the connection:

```httpdsl
conn.close()
```

## Parameter Placeholders

**PostgreSQL**: Use `$1`, `$2`, etc.

```httpdsl
conn = db.open("postgres", "...")
user = conn.query_one(
  "SELECT * FROM users WHERE id = $1",
  [1]
)
```

**MySQL/SQLite**: Use `?`

```httpdsl
conn = db.open("sqlite", "./app.db")
user = conn.query_one(
  "SELECT * FROM users WHERE id = ?",
  [1]
)
```

## Complete CRUD Example

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

db_conn.exec(`
  CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    created_at TEXT
  )
`, [])

route GET "/users" {
  users = db_conn.query("SELECT * FROM users", [])
  response.body = {users: users}
}

route GET "/users/:id" {
  user_id = int(request.params.id)
  
  user = db_conn.query_one(
    "SELECT * FROM users WHERE id = ?",
    [user_id]
  )
  
  if user == null {
    response.status = 404
    response.body = {error: "User not found"}
  } else {
    response.body = user
  }
}

route POST "/users" json {
  {name, email} = request.data
  
  if name == "" || email == "" {
    response.status = 400
    response.body = {error: "Name and email required"}
    return
  }
  
  try {
    result = db_conn.exec(
      "INSERT INTO users (name, email, created_at) VALUES (?, ?, ?)",
      [name, email, date()]
    )
    
    user = db_conn.query_one(
      "SELECT * FROM users WHERE id = ?",
      [result.last_insert_id]
    )
    
    response.status = 201
    response.body = user
  } catch err {
    response.status = 409
    response.body = {error: "Email already exists"}
  }
}

route PUT "/users/:id" json {
  user_id = int(request.params.id)
  {name, email} = request.data
  
  if name == "" || email == "" {
    response.status = 400
    response.body = {error: "Name and email required"}
    return
  }
  
  result = db_conn.exec(
    "UPDATE users SET name = ?, email = ? WHERE id = ?",
    [name, email, user_id]
  )
  
  if result.rows_affected == 0 {
    response.status = 404
    response.body = {error: "User not found"}
  } else {
    user = db_conn.query_one(
      "SELECT * FROM users WHERE id = ?",
      [user_id]
    )
    response.body = user
  }
}

route DELETE "/users/:id" {
  user_id = int(request.params.id)
  
  result = db_conn.exec(
    "DELETE FROM users WHERE id = ?",
    [user_id]
  )
  
  if result.rows_affected == 0 {
    response.status = 404
    response.body = {error: "User not found"}
  } else {
    response.body = {deleted: true}
  }
}
```

## MongoDB Operations

### find()

```httpdsl
conn = db.open("mongo", "mongodb://localhost:27017")

users = conn.find("users", {age: {"$gt": 18}})

each user in users {
  log_info(user.name)
}
```

### find_one()

```httpdsl
user = conn.find_one("users", {email: "alice@example.com"})

if user != null {
  log_info(user.name)
}
```

### insert()

```httpdsl
result = conn.insert("users", {
  name: "Alice",
  email: "alice@example.com",
  age: 30
})

log_info(`Inserted ID: ${result.inserted_id}`)
```

### update()

```httpdsl
result = conn.update(
  "users",
  {email: "alice@example.com"},
  {"$set": {age: 31}}
)

log_info(`Modified: ${result.modified_count}`)
```

### delete()

```httpdsl
result = conn.delete("users", {age: {"$lt": 18}})

log_info(`Deleted: ${result.deleted_count}`)
```

### count()

```httpdsl
count = conn.count("users", {active: true})
```

## MongoDB CRUD Example

```httpdsl
server {
  port 3000
}

mongo = db.open("mongo", "mongodb://localhost:27017/myapp")

route GET "/products" {
  products = mongo.find("products", {})
  response.body = {products: products}
}

route GET "/products/:id" {
  product_id = request.params.id
  
  product = mongo.find_one("products", {_id: product_id})
  
  if product == null {
    response.status = 404
    response.body = {error: "Product not found"}
  } else {
    response.body = product
  }
}

route POST "/products" json {
  {name, price, category} = request.data
  
  result = mongo.insert("products", {
    name: name,
    price: price,
    category: category,
    created_at: date()
  })
  
  response.status = 201
  response.body = {id: result.inserted_id}
}

route PUT "/products/:id" json {
  product_id = request.params.id
  {name, price} = request.data
  
  result = mongo.update(
    "products",
    {_id: product_id},
    {"$set": {name: name, price: price}}
  )
  
  if result.modified_count == 0 {
    response.status = 404
    response.body = {error: "Product not found"}
  } else {
    response.body = {updated: true}
  }
}

route DELETE "/products/:id" {
  product_id = request.params.id
  
  result = mongo.delete("products", {_id: product_id})
  
  if result.deleted_count == 0 {
    response.status = 404
    response.body = {error: "Product not found"}
  } else {
    response.body = {deleted: true}
  }
}
```

## Connection Pooling

Connections are reused across requests:

```httpdsl
db_conn = db.open("sqlite", "./app.db")

route GET "/data" {
  rows = db_conn.query("SELECT * FROM data", [])
  response.body = {rows: rows}
}
```

## Transactions

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

route POST "/transfer" json {
  {from_id, to_id, amount} = request.data
  
  try {
    db_conn.exec("BEGIN TRANSACTION", [])
    
    db_conn.exec(
      "UPDATE accounts SET balance = balance - ? WHERE id = ?",
      [amount, from_id]
    )
    
    db_conn.exec(
      "UPDATE accounts SET balance = balance + ? WHERE id = ?",
      [amount, to_id]
    )
    
    db_conn.exec("COMMIT", [])
    
    response.body = {success: true}
  } catch err {
    db_conn.exec("ROLLBACK", [])
    
    response.status = 500
    response.body = {error: "Transaction failed"}
  }
}
```

## Environment Configuration

```httpdsl
db_url = env("DATABASE_URL", "sqlite::memory:")
db_type = env("DB_TYPE", "sqlite")

conn = db.open(db_type, db_url)

route GET "/stats" {
  count = conn.query_value("SELECT COUNT(*) FROM users", [])
  response.body = {user_count: count}
}
```
