# Encoding Builtins

Functions for encoding and decoding data.

## base64_encode()

Encode string to Base64:

```httpdsl
base64_encode("hello")
base64_encode("Hello, World!")
base64_encode("user:password")
```

## base64_decode()

Decode Base64 string:

```httpdsl
base64_decode("aGVsbG8=")
base64_decode("SGVsbG8sIFdvcmxkIQ==")
```

## url_encode()

URL encode string:

```httpdsl
url_encode("hello world")
url_encode("user@example.com")
url_encode("a=1&b=2")
```

## url_decode()

URL decode string:

```httpdsl
url_decode("hello%20world")
url_decode("user%40example.com")
url_decode("a%3D1%26b%3D2")
```

## json.parse()

Parse JSON string:

```httpdsl
json.parse('{"name":"Alice","age":30}')
json.parse('[1,2,3]')
json.parse('"hello"')
```

## json.stringify()

Convert to JSON string:

```httpdsl
json.stringify({name: "Alice", age: 30})
json.stringify([1, 2, 3])
json.stringify("hello")
```

## Complete Examples

### Basic Auth Header

```httpdsl
route GET "/protected" {
  auth = request.basic
  
  if auth == null {
    response.status = 401
    response.headers = {
      "WWW-Authenticate": 'Basic realm="Restricted"'
    }
    response.body = {error: "Authentication required"}
    return
  }
  
  response.body = {
    username: auth.username,
    decoded: true
  }
}
```

### Create Basic Auth Header

```httpdsl
route POST "/create-auth" json {
  {username, password} = request.data
  
  credentials = `${username}:${password}`
  encoded = base64_encode(credentials)
  header = `Basic ${encoded}`
  
  response.body = {
    header: header,
    encoded: encoded
  }
}
```

### URL Builder

```httpdsl
route POST "/build-url" json {
  base = request.data.base ?? "https://api.example.com"
  params = request.data.params ?? {}
  
  query_parts = []
  
  each key in keys(params) {
    encoded_key = url_encode(key)
    encoded_value = url_encode(str(params[key]))
    query_parts = append(query_parts, `${encoded_key}=${encoded_value}`)
  }
  
  query_string = join(query_parts, "&")
  
  full_url = base
  if query_string != "" {
    full_url = `${base}?${query_string}`
  }
  
  response.body = {
    url: full_url,
    query: query_string
  }
}
```

### Parse Query String

```httpdsl
route POST "/parse-query" json {
  query = request.data.query ?? ""
  
  pairs = split(query, "&")
  params = {}
  
  each pair in pairs {
    if contains(pair, "=") {
      parts = split(pair, "=")
      if len(parts) == 2 {
        key = url_decode(parts[0])
        value = url_decode(parts[1])
        params[key] = value
      }
    }
  }
  
  response.body = params
}
```

### JSON Storage

```httpdsl
route POST "/store" json {
  data = request.data
  key = data.key ?? cuid2()
  
  json_string = json.stringify(data.value)
  store.set(key, json_string)
  
  response.body = {key: key, stored: true}
}

route GET "/retrieve/:key" {
  key = request.params.key
  json_string = store.get(key)
  
  if json_string == null {
    response.status = 404
    response.body = {error: "Not found"}
    return
  }
  
  data = json.parse(json_string)
  response.body = {key: key, value: data}
}
```

### Data Export

```httpdsl
route GET "/export" {
  users = [
    {id: 1, name: "Alice"},
    {id: 2, name: "Bob"}
  ]
  
  format = request.query.format ?? "json"
  
  if format == "json" {
    response.type = "json"
    response.body = users
  } else if format == "base64" {
    json_str = json.stringify(users)
    encoded = base64_encode(json_str)
    
    response.type = "text"
    response.body = encoded
  } else {
    response.status = 400
    response.body = {error: "Unsupported format"}
  }
}
```

### Webhook Signature

```httpdsl
route POST "/webhook" json {
  payload = request.data
  secret = env("WEBHOOK_SECRET")
  
  payload_json = json.stringify(payload)
  signature = hmac_hash("sha256", secret, payload_json)
  signature_b64 = base64_encode(signature)
  
  response.headers = {
    "X-Webhook-Signature": signature_b64
  }
  
  response.body = {
    received: true,
    signature: signature_b64
  }
}
```

### Data Encoding API

```httpdsl
route POST "/encode" json {
  text = request.data.text ?? ""
  encoding = request.data.encoding ?? "base64"
  
  result = ""
  
  switch encoding {
    case "base64" {
      result = base64_encode(text)
    }
    case "url" {
      result = url_encode(text)
    }
    case "json" {
      result = json.stringify(text)
    }
    default {
      response.status = 400
      response.body = {error: "Unsupported encoding"}
      return
    }
  }
  
  response.body = {
    original: text,
    encoding: encoding,
    encoded: result
  }
}

route POST "/decode" json {
  text = request.data.text ?? ""
  encoding = request.data.encoding ?? "base64"
  
  result = ""
  
  try {
    switch encoding {
      case "base64" {
        result = base64_decode(text)
      }
      case "url" {
        result = url_decode(text)
      }
      case "json" {
        result = json.parse(text)
      }
      default {
        response.status = 400
        response.body = {error: "Unsupported encoding"}
        return
      }
    }
    
    response.body = {
      original: text,
      encoding: encoding,
      decoded: result
    }
  } catch(err) {
    response.status = 400
    response.body = {error: "Decoding failed"}
  }
}
```

### Safe JSON Parsing

```httpdsl
fn safe_json_parse(str, default_value) {
  try {
    return json.parse(str)
  } catch(err) {
    return default_value
  }
}

route POST "/parse-safe" json {
  json_string = request.data.json ?? ""
  
  parsed = safe_json_parse(json_string, {})
  
  response.body = {
    parsed: parsed,
    type: type(parsed)
  }
}
```
