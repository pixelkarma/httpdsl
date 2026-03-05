# File Operations

HTTPDSL provides built-in functions for file system operations.

## Reading Files

### file.read()

Read text files:

```httpdsl
content = file.read("./data.txt")
log_info(content)
```

In route handlers:

```httpdsl
route GET "/readme" {
  content = file.read("./README.md")
  response.type = "text"
  response.body = content
}
```

### file.read_json()

Read and parse JSON files:

```httpdsl
config = file.read_json("./config.json")
log_info(config.version)
```

With error handling:

```httpdsl
route GET "/config" {
  try {
    config = file.read_json("./config.json")
    response.body = config
  } catch err {
    response.status = 500
    response.body = {error: "Failed to load configuration"}
  }
}
```

## Writing Files

### file.write()

Write text to file:

```httpdsl
file.write("./output.txt", "Hello, World!")
```

Overwrites existing content:

```httpdsl
route POST "/save" json {
  content = request.data.content
  filename = request.data.filename
  
  file.write(`./uploads/${filename}`, content)
  
  response.body = {saved: filename}
}
```

### file.append()

Append to file:

```httpdsl
file.append("./log.txt", `${date()}: New entry\n`)
```

Logging example:

```httpdsl
route POST "/log" json {
  message = request.data.message
  log_entry = `${date()} - ${message}\n`
  
  file.append("./app.log", log_entry)
  
  response.body = {logged: true}
}
```

### file.write_json()

Write data as JSON:

```httpdsl
data = {
  version: "1.0",
  settings: {theme: "dark"}
}

file.write_json("./config.json", data)
```

Pretty-printed with indentation:

```httpdsl
route POST "/export" json {
  data = request.data
  
  file.write_json("./export.json", data)
  
  response.body = {exported: true}
}
```

## File Checks

### file.exists()

Check if file exists:

```httpdsl
if file.exists("./config.json") {
  config = file.read_json("./config.json")
} else {
  config = {default: true}
}
```

In routes:

```httpdsl
route GET "/files/:name" {
  filename = request.params.name
  path = `./files/${filename}`
  
  if !file.exists(path) {
    response.status = 404
    response.body = {error: "File not found"}
    return
  }
  
  content = file.read(path)
  response.body = content
}
```

## File Management

### file.delete()

Delete a file:

```httpdsl
file.delete("./temp.txt")
```

With error handling:

```httpdsl
route DELETE "/files/:name" {
  filename = request.params.name
  path = `./uploads/${filename}`
  
  if !file.exists(path) {
    response.status = 404
    response.body = {error: "File not found"}
    return
  }
  
  try {
    file.delete(path)
    response.body = {deleted: filename}
  } catch err {
    response.status = 500
    response.body = {error: "Failed to delete file"}
  }
}
```

### file.list()

List directory contents:

```httpdsl
files = file.list("./uploads")

each filename in files {
  log_info(filename)
}
```

With filtering:

```httpdsl
route GET "/files" {
  all_files = file.list("./uploads")
  json_files = []
  
  each filename in all_files {
    if ends_with(filename, ".json") {
      json_files = append(json_files, filename)
    }
  }
  
  response.body = {files: json_files}
}
```

### file.mkdir()

Create directory:

```httpdsl
file.mkdir("./uploads")
file.mkdir("./data/cache")
```

Ensure directory exists:

```httpdsl
route POST "/upload" json {
  if !file.exists("./uploads") {
    file.mkdir("./uploads")
  }
  
  filename = request.data.filename
  content = request.data.content
  
  file.write(`./uploads/${filename}`, content)
  
  response.body = {uploaded: filename}
}
```

### file.chmod()

Change file permissions:

```httpdsl
file.chmod("./script.sh", "0755")
file.chmod("./config.json", "0644")
```

## Complete Examples

### Simple File API

```httpdsl
server {
  port 3000
}

base_path = "./storage"

if !file.exists(base_path) {
  file.mkdir(base_path)
}

route GET "/files" {
  files = file.list(base_path)
  response.body = {files: files}
}

route GET "/files/:name" {
  filename = request.params.name
  path = `${base_path}/${filename}`
  
  if !file.exists(path) {
    response.status = 404
    response.body = {error: "File not found"}
    return
  }
  
  content = file.read(path)
  response.body = content
}

route POST "/files/:name" text {
  filename = request.params.name
  content = request.data
  path = `${base_path}/${filename}`
  
  file.write(path, content)
  
  response.status = 201
  response.body = {created: filename}
}

route DELETE "/files/:name" {
  filename = request.params.name
  path = `${base_path}/${filename}`
  
  if !file.exists(path) {
    response.status = 404
    response.body = {error: "File not found"}
    return
  }
  
  file.delete(path)
  response.body = {deleted: filename}
}
```

### Configuration Manager

```httpdsl
server {
  port 3000
}

config_file = "./config.json"

fn load_config() {
  if file.exists(config_file) {
    return file.read_json(config_file)
  }
  
  default_config = {
    app_name: "My App",
    version: "1.0.0",
    debug: false
  }
  
  file.write_json(config_file, default_config)
  return default_config
}

fn save_config(config) {
  file.write_json(config_file, config)
}

config = load_config()

route GET "/config" {
  response.body = config
}

route PUT "/config" json {
  {app_name, version, debug} = request.data
  
  if app_name != null {
    config.app_name = app_name
  }
  
  if version != null {
    config.version = version
  }
  
  if debug != null {
    config.debug = debug
  }
  
  save_config(config)
  
  response.body = config
}
```

### Log Viewer

```httpdsl
server {
  port 3000
}

log_file = "./app.log"

route GET "/logs" {
  if !file.exists(log_file) {
    response.body = {logs: []}
    return
  }
  
  content = file.read(log_file)
  lines = split(content, "\n")
  
  response.body = {logs: lines, count: len(lines)}
}

route POST "/logs" json {
  message = request.data.message
  entry = `${date()}: ${message}\n`
  
  file.append(log_file, entry)
  
  response.body = {logged: true}
}

route DELETE "/logs" {
  if file.exists(log_file) {
    file.delete(log_file)
  }
  
  response.body = {cleared: true}
}
```

### Data Backup

```httpdsl
server {
  port 3000
}

db_conn = db.open("sqlite", "./app.db")

route POST "/backup" {
  users = db_conn.query("SELECT * FROM users", [])
  
  backup_data = {
    timestamp: date(),
    users: users
  }
  
  filename = `backup_${date("unix")}.json`
  
  if !file.exists("./backups") {
    file.mkdir("./backups")
  }
  
  file.write_json(`./backups/${filename}`, backup_data)
  
  response.body = {
    backup_file: filename,
    records: len(users)
  }
}

route GET "/backups" {
  if !file.exists("./backups") {
    response.body = {backups: []}
    return
  }
  
  files = file.list("./backups")
  backups = []
  
  each filename in files {
    if ends_with(filename, ".json") {
      backups = append(backups, filename)
    }
  }
  
  response.body = {backups: backups}
}

route POST "/restore/:filename" {
  filename = request.params.filename
  path = `./backups/${filename}`
  
  if !file.exists(path) {
    response.status = 404
    response.body = {error: "Backup not found"}
    return
  }
  
  backup = file.read_json(path)
  restored = 0
  
  each user in backup.users {
    try {
      db_conn.exec(
        "INSERT OR REPLACE INTO users (id, name, email) VALUES (?, ?, ?)",
        [user.id, user.name, user.email]
      )
      restored += 1
    } catch err {
      log_error(`Failed to restore user ${user.id}: ${err}`)
    }
  }
  
  response.body = {
    restored: restored,
    total: len(backup.users)
  }
}
```
