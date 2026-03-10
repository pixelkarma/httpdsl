# File Operations

- [File Handles](#file-handles)
  - [Handle Methods](#handle-methods)
  - [Passing Handles to Functions](#passing-handles-to-functions)
  - [Line-Based Reading](#line-based-reading)
- [Namespace Utilities](#namespace-utilities)
- [Reading Files](#reading-files)
  - [file.read()](#fileread)
  - [file.read_json()](#fileread_json)
- [Writing Files](#writing-files)
  - [file.write()](#filewrite)
  - [file.append()](#fileappend)
  - [file.write_json()](#filewrite_json)
- [File Checks](#file-checks)
  - [file.exists()](#fileexists)
- [File Management](#file-management)
  - [file.delete()](#filedelete)
  - [file.list()](#filelist)
  - [file.mkdir()](#filemkdir)
  - [file.chmod()](#filechmod)
- [Complete Examples](#complete-examples)
  - [Simple File API](#simple-file-api)
  - [Configuration Manager](#configuration-manager)
  - [Log Viewer](#log-viewer)
  - [Data Backup](#data-backup)

HTTPDSL provides built-in functions for file system operations. There are two styles:

- **File handles** — `file.open(path)` returns a handle for repeated access to the same file
- **Namespace utilities** — `file.read(path)`, `file.write(path, data)`, etc. for quick one-liners

## File Handles

Open a file handle with `file.open()`. The handle stores the path and can be passed to functions:

```httpdsl
f = file.open("data.txt")
f.write("hello")
f.append(" world\n")
data = f.read()           // "hello world\n"
```

File handles don't hold an OS file descriptor — each method opens, operates, and closes internally. No `.close()` needed. Safe for concurrent requests.

### Handle Methods

| Method | Description |
|--------|-------------|
| `f.read()` | Read entire file as string |
| `f.write(data)` | Overwrite file contents |
| `f.append(data)` | Append to file |
| `f.lines()` | All lines as array |
| `f.lines(n)` | First `n` lines |
| `f.lines(-n)` | Last `n` lines |
| `f.lines(start, end)` | Lines from `start` to `end` (slice) |
| `f.json()` | Read and parse as JSON |
| `f.write_json(obj)` | Serialize and write JSON |
| `f.exists()` | Check if file exists |
| `f.size()` | File size in bytes |
| `f.delete()` | Delete the file |
| `f.path()` | Return the file path |

### Passing Handles to Functions

```httpdsl
fn log_to(f, message) {
  f.append(message + "\n")
}

fn tail(f, n) {
  return f.lines(0 - n)
}

route POST "/log" {
  logfile = file.open("./app.log")
  log_to(logfile, request.data.message)
  response.body = {last10: tail(logfile, 10)}
}
```

### Line-Based Reading

```httpdsl
f = file.open("access.log")

all_lines = f.lines()        // every line as array
first_10 = f.lines(10)       // first 10 lines
last_5 = f.lines(-5)         // last 5 lines
chunk = f.lines(100, 200)    // lines 100-199
```

## Namespace Utilities

For quick one-off operations without creating a handle:

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
  } catch(err) {
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
file.append("./log.txt", `${date_format(now(), "2006-01-02T15:04:05Z")}: New entry\n`)
```

Logging example:

```httpdsl
route POST "/log" json {
  message = request.data.message
  log_entry = `${date_format(now(), "2006-01-02T15:04:05Z")} - ${message}\n`
  
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
  } catch(err) {
    response.status = 500
    response.body = {error: "Failed to delete file"}
  }
}
```

### file.list()

List directory contents. Returns an array of objects with `name`, `is_dir`, and `size` fields:

```httpdsl
entries = file.list("./uploads")

each entry in entries {
  log_info(entry.name + " (" + str(entry.size) + " bytes)")
}
```

With filtering:

```httpdsl
route GET "/files" {
  all_entries = file.list("./uploads")
  json_files = []
  
  each entry in all_entries {
    if ends_with(entry.name, ".json") {
      json_files = append(json_files, entry.name)
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
file.chmod("./script.sh", 0755)
file.chmod("./config.json", 0644)
```

## Complete Examples

### Simple File API

```httpdsl
server {
  port 3000
}

init {
  base_path = "./storage"

  if !file.exists(base_path) {
    file.mkdir(base_path)
  }
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

init {
  config_file = "./config.json"
}

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

init {
  config = load_config()
}

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

Using a file handle for repeated access to the same log file:

```httpdsl
server {
  port 3000
}

init {
  log = file.open("./app.log")
}

route GET "/logs" {
  if !log.exists() {
    response.body = {logs: []}
    return
  }

  // Query params for pagination
  n = int(request.query.last ?? "50")
  lines = log.lines(0 - n)

  response.body = {logs: lines, count: len(lines)}
}

route GET "/logs/tail" {
  response.body = {last10: log.lines(-10)}
}

route POST "/logs" json {
  message = request.data.message
  entry = `${date_format(now(), "2006-01-02T15:04:05Z")}: ${message}\n`

  log.append(entry)

  response.body = {logged: true}
}

route DELETE "/logs" {
  if log.exists() {
    log.delete()
  }

  response.body = {cleared: true}
}
```

### Data Backup

```httpdsl
server {
  port 3000
}

init {
  db_conn = db.open("sqlite", "./app.db")
}

route POST "/backup" {
  users = db_conn.query("SELECT * FROM users", [])
  
  backup_data = {
    timestamp: now(),
    users: users
  }
  
  filename = `backup_${now()}.json`
  
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
  
  entries = file.list("./backups")
  backups = []
  
  each entry in entries {
    if ends_with(entry.name, ".json") {
      backups = append(backups, entry.name)
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
    } catch(err) {
      log_error(`Failed to restore user ${user.id}: ${err}`)
    }
  }
  
  response.body = {
    restored: restored,
    total: len(backup.users)
  }
}
```
