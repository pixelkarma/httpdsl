# Exec Builtin

Execute shell commands from HTTPDSL.

## Basic Usage

```httpdsl
exec("ls -la")
exec("echo hello")
exec("date")
```

Returns:
```httpdsl
{
  stdout: "...",
  stderr: "...",
  status: 0,
  ok: true
}
```

- `stdout`: Standard output as string
- `stderr`: Standard error as string
- `status`: Exit code (0 = success, -1 = timeout)
- `ok`: Boolean (true if status == 0)

## With Timeout

```httpdsl
exec("sleep 5", 10)
exec("long-running-command", 30)
```

Default timeout: 30 seconds

## Command Execution

Commands run via `sh -c`, so shell features work:

```httpdsl
exec("ls *.txt")
exec("echo $HOME")
exec("cat file.txt | grep pattern")
```

## Complete Examples

### System Information

```httpdsl
route GET "/system-info" {
  uname = exec("uname -a")
  uptime = exec("uptime")
  df = exec("df -h")
  
  response.body = {
    system: trim(uname.stdout),
    uptime: trim(uptime.stdout),
    disk: trim(df.stdout)
  }
}
```

### Git Operations

```httpdsl
route GET "/git/status" {
  result = exec("git status --short")
  
  if !result.ok {
    response.status = 500
    response.body = {error: result.stderr}
    return
  }
  
  response.body = {output: result.stdout}
}

route POST "/git/commit" json {
  message = request.data.message ?? "Update"
  
  add = exec("git add .")
  
  if !add.ok {
    response.status = 500
    response.body = {error: "Failed to stage changes"}
    return
  }
  
  commit = exec(`git commit -m "${message}"`)
  
  response.body = {
    success: commit.ok,
    output: commit.stdout,
    error: commit.stderr
  }
}
```

### File Processing

```httpdsl
route GET "/count-lines" {
  filename = request.query.file ?? "data.txt"
  
  result = exec(`wc -l ${filename}`)
  
  if !result.ok {
    response.status = 404
    response.body = {error: "File not found"}
    return
  }
  
  lines = int(trim(split(result.stdout, " ")[0]))
  
  response.body = {file: filename, lines: lines}
}
```

### Directory Listing

```httpdsl
route GET "/files" {
  result = exec("ls -la")
  
  if !result.ok {
    response.status = 500
    response.body = {error: result.stderr}
    return
  }
  
  lines = split(result.stdout, "\n")
  
  response.body = {files: lines}
}
```

### Backup Creation

```httpdsl
route POST "/backup" {
  timestamp = now()
  filename = `backup_${timestamp}.tar.gz`
  
  result = exec(`tar -czf /backups/${filename} ./data`, 60)
  
  if !result.ok {
    response.status = 500
    response.body = {error: "Backup failed", stderr: result.stderr}
    return
  }
  
  response.body = {backup: filename, created: true}
}
```

### Async Execution

```httpdsl
route GET "/async-exec" {
  f1 = async exec("command1")
  f2 = async exec("command2")
  f3 = async exec("command3")
  
  r1, r2, r3 = await(f1, f2, f3)
  
  response.body = {
    command1: {ok: r1.ok, output: r1.stdout},
    command2: {ok: r2.ok, output: r2.stdout},
    command3: {ok: r3.ok, output: r3.stdout}
  }
}
```

### Image Processing

```httpdsl
route POST "/resize-image" json {
  input = request.data.input ?? ""
  output = request.data.output ?? ""
  width = int(request.data.width ?? "800")
  
  result = exec(`convert ${input} -resize ${width} ${output}`, 30)
  
  if !result.ok {
    response.status = 500
    response.body = {error: "Image processing failed"}
    return
  }
  
  response.body = {processed: output}
}
```

### Health Check

```httpdsl
route GET "/health" {
  checks = {
    disk: exec("df -h /"),
    memory: exec("free -m"),
    processes: exec("ps aux | wc -l")
  }
  
  all_ok = checks.disk.ok && checks.memory.ok && checks.processes.ok
  
  response.status = all_ok ? 200 : 503
  response.body = {
    healthy: all_ok,
    checks: {
      disk: checks.disk.ok,
      memory: checks.memory.ok,
      processes: checks.processes.ok
    }
  }
}
```

### Database Dump

```httpdsl
route POST "/db/dump" {
  timestamp = now()
  filename = `dump_${timestamp}.sql`
  
  result = exec(`mysqldump -u user -ppassword dbname > /backups/${filename}`, 120)
  
  if !result.ok {
    response.status = 500
    response.body = {error: "Database dump failed"}
    return
  }
  
  response.body = {dump: filename}
}
```

### Log Analysis

```httpdsl
route GET "/logs/errors" {
  result = exec(`grep ERROR /var/log/app.log | tail -n 100`)
  
  if !result.ok {
    response.status = 500
    response.body = {error: "Failed to read logs"}
    return
  }
  
  lines = split(result.stdout, "\n")
  filtered = filter(lines, fn(line) { return line != "" })
  
  response.body = {
    errors: filtered,
    count: len(filtered)
  }
}
```

### Network Check

```httpdsl
route GET "/ping/:host" {
  host = request.params.host
  
  result = exec(`ping -c 4 ${host}`, 10)
  
  response.body = {
    host: host,
    reachable: result.ok,
    output: result.stdout
  }
}
```

### Timeout Handling

```httpdsl
route POST "/long-task" {
  result = exec("sleep 60", 5)
  
  if result.status == -1 {
    response.status = 504
    response.body = {error: "Command timed out"}
    return
  }
  
  response.body = {completed: result.ok}
}
```

## Security Considerations

**Warning**: `exec()` runs shell commands. Be careful with user input:

```httpdsl
route GET "/unsafe" {
  filename = request.query.file
  
  result = exec(`cat ${filename}`)
}
```

Validate and sanitize input:

```httpdsl
route GET "/safe" {
  filename = request.query.file ?? ""
  
  if contains(filename, "..") || contains(filename, "/") {
    response.status = 400
    response.body = {error: "Invalid filename"}
    return
  }
  
  allowed = ["data.txt", "info.txt", "readme.txt"]
  
  if !contains(allowed, filename) {
    response.status = 403
    response.body = {error: "File not allowed"}
    return
  }
  
  result = exec(`cat /safe/path/${filename}`)
  
  response.body = {content: result.stdout}
}
```
