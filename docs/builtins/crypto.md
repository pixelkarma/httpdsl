# Crypto Builtins

Cryptographic and hashing functions.

## hash()

Generate hash digest:

```httpdsl
hash("sha256", "hello")
hash("sha512", "data")
hash("md5", "test")
```

Supported algorithms: `sha256`, `sha512`, `md5`

## hmac()

Generate HMAC:

```httpdsl
hmac("sha256", "message", "secret-key")
hmac("sha512", "data", "key")
hmac("sha256", "payload", "webhook-secret")
```

## uuid()

Generate UUID v4:

```httpdsl
uuid()
uuid()
uuid()
```

## cuid2()

Generate CUID2 (collision-resistant unique ID):

```httpdsl
cuid2()
cuid2()
cuid2(24)
```

Optional length parameter (default is standard CUID2 length).

## Complete Examples

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

### Webhook Verification

```httpdsl
server {
  port 3000
}

webhook_secret = env("WEBHOOK_SECRET")

route POST "/webhook" json {
  signature = request.headers["x-webhook-signature"] ?? ""
  payload = request.data
  
  payload_json = json.stringify(payload)
  expected = hmac("sha256", payload_json, webhook_secret)
  
  if signature != expected {
    response.status = 401
    response.body = {error: "Invalid signature"}
    return
  }
  
  log_info("Valid webhook received")
  
  response.body = {received: true}
}
```

### API Request Signing

```httpdsl
route POST "/api/sign-request" json {
  method = request.data.method ?? "GET"
  path = request.data.path ?? "/"
  body = request.data.body ?? ""
  api_secret = env("API_SECRET")
  
  timestamp = str(date("unix"))
  nonce = cuid2(16)
  
  string_to_sign = `${method}\n${path}\n${timestamp}\n${nonce}\n${body}`
  
  signature = hmac("sha256", string_to_sign, api_secret)
  
  response.body = {
    method: method,
    path: path,
    timestamp: timestamp,
    nonce: nonce,
    signature: signature
  }
}
```

### Unique ID Generation

```httpdsl
route POST "/api/users" json {
  {name, email} = request.data
  
  user = {
    id: cuid2(),
    uuid: uuid(),
    name: name,
    email: email,
    created_at: date()
  }
  
  response.status = 201
  response.body = user
}
```

### Password Reset Token

```httpdsl
route POST "/auth/forgot-password" json {
  email = request.data.email ?? ""
  
  token = cuid2(32)
  expires_at = date("unix") + 3600
  
  token_hash = hash("sha256", token)
  
  store.set(`reset:${token_hash}`, {
    email: email,
    expires_at: expires_at
  }, 3600)
  
  response.body = {
    token: token,
    message: "Reset token generated"
  }
}

route POST "/auth/reset-password" json {
  token = request.data.token ?? ""
  new_password = request.data.password ?? ""
  
  token_hash = hash("sha256", token)
  
  reset_data = store.get(`reset:${token_hash}`)
  
  if reset_data == null {
    response.status = 401
    response.body = {error: "Invalid or expired token"}
    return
  }
  
  if reset_data.expires_at < date("unix") {
    response.status = 401
    response.body = {error: "Token expired"}
    return
  }
  
  store.delete(`reset:${token_hash}`)
  
  response.body = {message: "Password reset successful"}
}
```

### Content Hash Caching

```httpdsl
route GET "/content/:hash" {
  content_hash = request.params.hash
  
  content = store.get(`content:${content_hash}`)
  
  if content == null {
    response.status = 404
    response.body = {error: "Content not found"}
    return
  }
  
  response.body = content
}

route POST "/content" json {
  content = request.data.content ?? ""
  
  content_hash = hash("sha256", content)
  
  store.set(`content:${content_hash}`, content, 86400)
  
  response.body = {
    hash: content_hash,
    url: `/content/${content_hash}`
  }
}
```

### ETag Generation

```httpdsl
route GET "/api/data" {
  data = {message: "Hello", timestamp: date()}
  
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
  
  api_key = cuid2(40)
  api_key_hash = hash("sha256", api_key)
  
  store.set(`apikey:${api_key_hash}`, {
    user_id: user_id,
    created_at: date()
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

### Session Token

```httpdsl
route POST "/auth/login" json {
  {username, password} = request.data
  
  if username == "admin" && password == "secret" {
    session_token = cuid2(32)
    session_hash = hash("sha256", session_token)
    
    store.set(`session:${session_hash}`, {
      username: username,
      created_at: date("unix")
    }, 3600)
    
    response.body = {token: session_token}
  } else {
    response.status = 401
    response.body = {error: "Invalid credentials"}
  }
}
```
