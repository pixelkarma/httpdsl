# Crypto Builtins

Cryptographic hashing, HMAC, password hashing, and unique ID generation.

## hash()

Generate a hex-encoded hash digest:

```httpdsl
hash("sha256", "hello")       // SHA-256 hex string
hash("sha512", "data")        // SHA-512 hex string
hash("md5", "test")           // MD5 hex string
```

**Signature:** `hash(algo, data)`

| Parameter | Type   | Description |
|-----------|--------|-------------|
| `algo`    | string | `"sha256"`, `"sha512"`, or `"md5"` |
| `data`    | string | The data to hash |

Returns a hex-encoded hash string. Returns `""` if the algorithm is unknown.

## hmac_hash()

Generate a hex-encoded HMAC:

```httpdsl
hmac_hash("sha256", "secret-key", "message")
hmac_hash("sha512", "key", "data")
```

**Signature:** `hmac_hash(algo, key, data)`

| Parameter | Type   | Description |
|-----------|--------|-------------|
| `algo`    | string | `"sha256"` or `"sha512"` |
| `key`     | string | The secret key |
| `data`    | string | The data to authenticate |

Returns a hex-encoded HMAC string. Returns `""` if the algorithm is unknown.

## hash_password()

Hash a password using bcrypt (default) or argon2:

```httpdsl
hash_password("mypassword")                          // bcrypt, default cost 12
hash_password("mypassword", "bcrypt")                 // bcrypt, explicit
hash_password("mypassword", "bcrypt", {cost: 14})     // bcrypt, custom cost
hash_password("mypassword", "argon2")                 // argon2 with defaults
hash_password("mypassword", "argon2", {               // argon2, custom opts
  memory: 65536,
  iterations: 3,
  parallelism: 4,
  key_length: 32
})
```

**Signature:** `hash_password(password)` / `hash_password(password, algo)` / `hash_password(password, algo, opts)`

| Parameter  | Type   | Description |
|------------|--------|-------------|
| `password` | string | The plaintext password |
| `algo`     | string | `"bcrypt"` (default) or `"argon2"` |
| `opts`     | object | Algorithm-specific options (see below) |

**bcrypt options:**

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `cost` | int  | `12`    | Work factor (range 4–31) |

**argon2 options:**

| Option        | Type | Default | Description |
|---------------|------|---------|-------------|
| `memory`      | int  | `65536` | Memory in KiB |
| `iterations`  | int  | `3`     | Number of iterations |
| `parallelism` | int  | `4`     | Degree of parallelism |
| `key_length`  | int  | `32`    | Length of derived key in bytes |

Returns the hashed password string.

## verify_password()

Verify a plaintext password against a hash:

```httpdsl
hashed = hash_password("secret")
verify_password("secret", hashed)    // true
verify_password("wrong", hashed)     // false
```

**Signature:** `verify_password(password, hash)`

| Parameter  | Type   | Description |
|------------|--------|-------------|
| `password` | string | The plaintext password to check |
| `hash`     | string | The stored hash to verify against |

Auto-detects bcrypt (`$2a$`, `$2b$`, `$2y$` prefixes) and argon2 (`$argon2id$` prefix). Returns `true` if the password matches, `false` otherwise.

## uuid()

Generate a random UUID v4:

```httpdsl
uuid()   // e.g. "550e8400-e29b-41d4-a716-446655440000"
```

**Signature:** `uuid()`

Takes no arguments. Returns a random UUID v4 string.

## cuid2()

Generate a CUID2 (collision-resistant unique identifier):

```httpdsl
cuid2()     // 24-character CUID2, e.g. "k8f3j2h1g0m4n5p6q7r8s9t0"
cuid2(32)   // custom length
```

**Signature:** `cuid2()` / `cuid2(length)`

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `length`  | int  | `24`    | Length of the generated ID |

Returns a CUID2 string. The first character is always a lowercase letter.

## Complete Examples

### Webhook Signature Verification

```httpdsl
server {
  port 3000
}

webhook_secret = env("WEBHOOK_SECRET")

route POST "/webhook" json {
  signature = request.headers["x-webhook-signature"] ?? ""
  payload = request.data

  payload_json = json.stringify(payload)
  expected = hmac_hash("sha256", webhook_secret, payload_json)

  if signature != expected {
    response.status = 401
    response.body = {error: "Invalid signature"}
    return
  }

  log_info("Valid webhook received")
  response.body = {received: true}
}
```

### User Registration with Password Hashing

```httpdsl
route POST "/auth/register" json {
  {username, password} = request.data

  hashed = hash_password(password)
  user_id = cuid2()

  store.set(`user:${user_id}`, {
    id: user_id,
    username: username,
    password_hash: hashed,
    created_at: now()
  })

  response.status = 201
  response.body = {id: user_id, username: username}
}

route POST "/auth/login" json {
  {username, password} = request.data

  // (lookup user by username)
  user = store.get(`user:${username}`)

  if user == null {
    response.status = 401
    response.body = {error: "Invalid credentials"}
    return
  }

  if verify_password(password, user.password_hash) {
    session_token = cuid2(32)
    session_hash = hash("sha256", session_token)

    store.set(`session:${session_hash}`, {
      user_id: user.id,
      created_at: now()
    }, 3600)

    response.body = {token: session_token}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```

### API Request Signing

```httpdsl
route POST "/api/sign-request" json {
  method = request.data.method ?? "GET"
  path = request.data.path ?? "/"
  body = request.data.body ?? ""
  api_secret = env("API_SECRET")

  timestamp = str(now())
  nonce = cuid2()

  string_to_sign = `${method}\n${path}\n${timestamp}\n${nonce}\n${body}`
  signature = hmac_hash("sha256", api_secret, string_to_sign)

  response.body = {
    method: method,
    path: path,
    timestamp: timestamp,
    nonce: nonce,
    signature: signature
  }
}
```

### File Integrity Check

```httpdsl
route POST "/upload" json {
  content = request.data.content ?? ""
  checksum = hash("sha256", content)
  file_id = cuid2()

  store.set(`file:${file_id}`, content)
  store.set(`checksum:${file_id}`, checksum)

  response.body = {
    id: file_id,
    checksum: checksum
  }
}

route GET "/verify/:id" {
  file_id = request.params.id
  content = store.get(`file:${file_id}`)
  stored_checksum = store.get(`checksum:${file_id}`)

  if content == null {
    response.status = 404
    response.body = {error: "File not found"}
    return
  }

  current_checksum = hash("sha256", content)

  response.body = {
    id: file_id,
    stored_checksum: stored_checksum,
    current_checksum: current_checksum,
    valid: stored_checksum == current_checksum
  }
}
```

### ETag Generation

```httpdsl
route GET "/api/data" {
  data = {message: "Hello", timestamp: now()}
  data_json = json.stringify(data)
  etag = hash("md5", data_json)

  client_etag = request.headers["if-none-match"] ?? ""

  if client_etag == etag {
    response.status = 304
    response.body = ""
    return
  }

  response.headers = {"ETag": etag}
  response.body = data
}
```

### API Key Generation

```httpdsl
route POST "/admin/generate-api-key" json {
  user_id = request.data.user_id

  api_key = uuid()
  api_key_hash = hash("sha256", api_key)

  store.set(`apikey:${api_key_hash}`, {
    user_id: user_id,
    created_at: now()
  })

  response.body = {
    api_key: api_key,
    message: "Save this key securely. It won't be shown again."
  }
}

route GET "/api/protected" {
  api_key = request.headers["x-api-key"] ?? ""

  if api_key == "" {
    response.status = 401
    response.body = {error: "API key required"}
    return
  }

  api_key_hash = hash("sha256", api_key)
  key_data = store.get(`apikey:${api_key_hash}`)

  if key_data == null {
    response.status = 401
    response.body = {error: "Invalid API key"}
    return
  }

  response.body = {
    user_id: key_data.user_id,
    data: "Protected data"
  }
}
```
