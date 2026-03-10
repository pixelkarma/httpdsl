# Server-Sent Events (SSE)

- [Basic SSE Route](#basic-sse-route)
- [stream — per-connection handle](#stream--per-connection-handle)
- [sse — global namespace](#sse--global-namespace)
- [Channel handles](#channel-handles)
- [Disconnect handler](#disconnect-handler)
- [Early Exit](#early-exit)
- [Chat Room Example](#chat-room-example)
- [Room-Based Chat](#room-based-chat)
- [Live Dashboard](#live-dashboard)
- [Notifications System](#notifications-system)
- [Progress Updates](#progress-updates)
- [Multiplayer Game State](#multiplayer-game-state)

SSE enables real-time server-to-client streaming over HTTP.

## Basic SSE Route

```httpdsl
server {
  port 3000
}

route SSE "/events" {
  stream.send("message", {text: "Hello from SSE!"})
}
```

Each SSE connection gets a unique **stream** handle, available as the `stream` variable inside the route body.

## stream — per-connection handle

The `stream` variable is automatically available inside every SSE route. It represents the current connection.

```httpdsl
stream.id                        // auto-assigned UUID
stream.set(key, value)           // per-connection metadata
stream.get(key)                  // read metadata
stream.send(type, data)          // send to this connection
stream.send(data)                // send with default type "message"
stream.join("channel")           // join a channel (O(1) indexed add)
stream.leave("channel")          // leave a channel
stream.close()                   // server-side disconnect
```

### stream.send()

Send events to the connected client:

```httpdsl
route SSE "/notifications" {
  stream.send("notification", {
    id: 1,
    message: "New notification",
    timestamp: now()
  })
}
```

With default event type `"message"`:

```httpdsl
route SSE "/updates" {
  stream.send({status: "connected"})
}
```

### stream.set() / stream.get()

Store per-connection metadata that persists for the lifetime of the connection:

```httpdsl
route SSE "/events" {
  stream.set("user", request.session.username)
  stream.set("role", "member")
  stream.send("welcome", {user: stream.get("user")})
}
```

Metadata is available in the disconnect handler and from other routes via `sse.find()` / `sse.find_by()`.

### stream.join() / stream.leave()

Join a named channel for targeted messaging:

```httpdsl
route SSE "/room/:id/events" {
  room_id = request.params.id
  stream.join(room_id)
  stream.send("join", {message: `Joined room ${room_id}`})
}
```

Channels use indexed lookups — no scanning of all connections.

### stream.id

Every stream gets a unique UUID, useful for direct messaging:

```httpdsl
route SSE "/events" {
  stream.send("welcome", {id: stream.id})
}

route POST "/send-to/:id" json {
  s = sse.find(request.params.id)
  if s != null {
    s.send("direct", request.data)
  }
  response.body = {ok: true}
}
```

### stream.close()

Disconnect a client from the server side:

```httpdsl
route POST "/kick/:id" {
  s = sse.find(request.params.id)
  if s != null {
    s.send("kicked", {reason: "Bye!"})
    s.close()
  }
  response.body = {ok: true}
}
```

## sse — global namespace

The `sse` namespace is usable from **any route** (SSE or regular).

```httpdsl
sse.find(id)                     // stream handle by UUID, or null
sse.find_by(key, value)          // array of stream handles matching metadata
sse.channel("name")              // channel handle
sse.broadcast(type, data)        // send to every connection
sse.count()                      // total live connections
```

### sse.broadcast()

Send to **all** connected SSE clients:

```httpdsl
route POST "/announce" json {
  sse.broadcast("announcement", {message: request.data.message})
  response.body = {sent: true}
}
```

### sse.find()

Look up a connection by its stream ID:

```httpdsl
route POST "/dm/:stream_id" json {
  target = sse.find(request.params.stream_id)
  if target != null {
    target.send("dm", {from: request.data.from, text: request.data.text})
  }
  response.body = {ok: target != null}
}
```

### sse.find_by()

Find all connections matching a metadata key/value pair:

```httpdsl
route POST "/notify-user/:username" json {
  streams = sse.find_by("user", request.params.username)
  each s in streams {
    s.send("notification", request.data)
  }
  response.body = {sent: len(streams)}
}
```

### sse.count()

```httpdsl
route GET "/stats" {
  response.body = {connections: sse.count()}
}
```

## Channel handles

`sse.channel("name")` returns a channel handle for targeted messaging.

```httpdsl
ch = sse.channel("room:123")
ch.send(type, data)              // send to all members (indexed, no scan)
ch.streams()                     // array of stream handles
ch.count()                       // member count
```

### Channel send

```httpdsl
route POST "/room/:id/message" json {
  room_id = request.params.id
  sse.channel(room_id).send("message", {
    username: request.data.username,
    text: request.data.text,
    timestamp: now()
  })
  response.body = {sent: true}
}
```

### Channel streams and count

```httpdsl
route GET "/room/:id/info" {
  ch = sse.channel(request.params.id)
  members = ch.streams()
  names = []
  each s in members {
    push(names, s.get("username"))
  }
  response.body = {
    count: ch.count(),
    members: names
  }
}
```

## Disconnect handler

SSE routes support an optional `disconnect` block that runs when a client disconnects. The disconnect block runs **after** the stream is removed from channels but **before** metadata is cleaned up, so `stream.get()` still works.

```httpdsl
route SSE "/events" {
  stream.set("name", request.session.username)
  stream.join("lobby")
  stream.send("welcome", {id: stream.id})
} disconnect {
  name = stream.get("name")
  sse.channel("lobby").send("left", {name: name})
}
```

## Early Exit

Use `return` to exit an SSE handler early (e.g., for auth checks):

```httpdsl
route SSE "/events/:key" {
  game = games[request.params.key]
  if game == null {
    return
  }
  stream.join(request.params.key)
  stream.send("state", {board: game.board})
}
```

`return` in an SSE route cleanly disconnects the client. SSE routes have no `response` object.

## Chat Room Example

```httpdsl
server {
  port 3000
}

route GET "/" {
  response.type = "html"
  response.body = `
    <!DOCTYPE html>
    <html>
      <head><title>Chat</title></head>
      <body>
        <div id="messages"></div>
        <input id="message" type="text" placeholder="Type a message">
        <button onclick="send()">Send</button>

        <script>
          const es = new EventSource('/events');
          es.addEventListener('message', (e) => {
            const data = JSON.parse(e.data);
            document.getElementById('messages').innerHTML +=
              '<div>' + data.text + '</div>';
          });
          function send() {
            const msg = document.getElementById('message').value;
            fetch('/send', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({message: msg})
            });
            document.getElementById('message').value = '';
          }
        </script>
      </body>
    </html>
  `
}

route SSE "/events" {
  stream.join("chat")
  stream.send({text: "Connected to chat"})
}

route POST "/send" json {
  sse.channel("chat").send("message", {text: request.data.message})
  response.body = {sent: true}
}
```

## Room-Based Chat

```httpdsl
server {
  port 3000
}

route SSE "/room/:id/events" {
  room_id = request.params.id
  username = request.query.username ?? "Anonymous"

  stream.set("username", username)
  stream.join(room_id)

  sse.channel(room_id).send("join", {
    username: username,
    message: `${username} joined the room`
  })

  stream.send("connected", {
    room: room_id,
    message: "You are connected"
  })
} disconnect {
  username = stream.get("username")
  room_id = stream.get("room")
  sse.channel(room_id).send("leave", {
    username: username,
    message: `${username} left the room`
  })
}

route POST "/room/:id/message" json {
  room_id = request.params.id
  {username, message} = request.data

  sse.channel(room_id).send("message", {
    username: username,
    message: message,
    timestamp: now()
  })

  response.body = {sent: true}
}
```

## Live Dashboard

```httpdsl
server {
  port 3000
}

route SSE "/dashboard/events" {
  stream.join("dashboard")
  stream.send("init", {message: "Dashboard connected"})
}

every 5 s {
  stats = server_stats()

  sse.channel("dashboard").send("stats", {
    memory: stats.mem_alloc_mb,
    goroutines: stats.goroutines,
    uptime: stats.uptime_human,
    connections: sse.count()
  })
}

route GET "/dashboard" {
  response.type = "html"
  response.body = `
    <!DOCTYPE html>
    <html>
      <head><title>Dashboard</title></head>
      <body>
        <h1>Server Stats</h1>
        <div id="stats"></div>

        <script>
          const es = new EventSource('/dashboard/events');
          es.addEventListener('stats', (e) => {
            const data = JSON.parse(e.data);
            document.getElementById('stats').innerHTML =
              '<p>Memory: ' + data.memory + ' MB</p>' +
              '<p>Goroutines: ' + data.goroutines + '</p>' +
              '<p>Uptime: ' + data.uptime + '</p>' +
              '<p>SSE Connections: ' + data.connections + '</p>';
          });
        </script>
      </body>
    </html>
  `
}
```

## Notifications System

```httpdsl
server {
  port 3000
  session {
    cookie "sid"
    expires 1 h
    secret env("SESSION_SECRET")
  }
}

route SSE "/notifications" {
  user_id = request.session.user_id

  if !user_id {
    stream.send("error", {message: "Authentication required"})
    return
  }

  stream.set("user_id", user_id)
  stream.join(`user:${user_id}`)

  stream.send("connected", {message: "Listening for notifications"})
}

route POST "/notify/:user_id" json {
  user_id = request.params.user_id

  sse.channel(`user:${user_id}`).send("notification", {
    message: request.data.message,
    timestamp: now()
  })

  response.body = {notified: user_id}
}
```

## Progress Updates

```httpdsl
server {
  port 3000
}

route SSE "/job/:id/progress" {
  job_id = request.params.id

  stream.join(`job:${job_id}`)

  stream.send("started", {job_id: job_id})
}

route POST "/job/:id/start" {
  job_id = request.params.id
  ch = sse.channel(`job:${job_id}`)

  steps = 10

  each i in range(steps) {
    sleep(1000)

    progress = int((float(i + 1) / float(steps)) * 100)

    ch.send("progress", {
      job_id: job_id,
      step: i + 1,
      total: steps,
      progress: progress
    })
  }

  ch.send("completed", {
    job_id: job_id,
    message: "Job completed"
  })

  response.body = {completed: true}
}
```

## Multiplayer Game State

```httpdsl
server {
  port 3000
}

route SSE "/game/:id/events" {
  game_id = request.params.id
  player_id = cuid2()

  stream.set("player_id", player_id)
  stream.join(game_id)

  stream.send("joined", {
    player_id: player_id,
    game_id: game_id
  })

  sse.channel(game_id).send("player_joined", {
    player_id: player_id
  })
} disconnect {
  player_id = stream.get("player_id")
  sse.channel(request.params.id).send("player_left", {
    player_id: player_id
  })
}

route POST "/game/:id/move" json {
  game_id = request.params.id
  {player_id, x, y} = request.data

  sse.channel(game_id).send("player_moved", {
    player_id: player_id,
    x: x,
    y: y
  })

  response.body = {moved: true}
}
```
