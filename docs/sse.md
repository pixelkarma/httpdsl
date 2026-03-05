# Server-Sent Events (SSE)

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

## stream.send()

Send events to the client:

```httpdsl
route SSE "/notifications" {
  stream.send("notification", {
    id: 1,
    message: "New notification",
    timestamp: date()
  })
}
```

With default event type:

```httpdsl
route SSE "/updates" {
  stream.send({status: "connected"})
}
```

This sends with event type "message".

## stream.join()

Join a channel for targeted broadcasts:

```httpdsl
route SSE "/room/:id" {
  room_id = request.params.id
  
  stream.join(room_id)
  
  stream.send("join", {message: `Joined room ${room_id}`})
}
```

## broadcast()

Broadcast to all connected clients:

```httpdsl
server {
  port 3000
}

route SSE "/events" {
  stream.send("connected", {message: "You are connected"})
}

route POST "/broadcast" json {
  message = request.data.message
  
  broadcast("notification", {message: message})
  
  response.body = {broadcasted: true}
}
```

Broadcast to specific channel:

```httpdsl
route POST "/room/:id/message" json {
  room_id = request.params.id
  message = request.data.message
  
  broadcast("message", {text: message}, room_id)
  
  response.body = {sent: true}
}
```

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
          const eventSource = new EventSource('/events');
          
          eventSource.addEventListener('message', (e) => {
            const data = JSON.parse(e.data);
            document.getElementById('messages').innerHTML += 
              '<div>' + data.text + '</div>';
          });
          
          function send() {
            const message = document.getElementById('message').value;
            fetch('/send', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({message: message})
            });
            document.getElementById('message').value = '';
          }
        </script>
      </body>
    </html>
  `
}

route SSE "/events" {
  stream.send({text: "Connected to chat"})
}

route POST "/send" json {
  message = request.data.message
  
  broadcast("message", {text: message})
  
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
  
  stream.join(room_id)
  
  broadcast("join", {
    username: username,
    message: `${username} joined the room`
  }, room_id)
  
  stream.send("connected", {
    room: room_id,
    message: "You are connected"
  })
}

route POST "/room/:id/message" json {
  room_id = request.params.id
  {username, message} = request.data
  
  broadcast("message", {
    username: username,
    message: message,
    timestamp: date()
  }, room_id)
  
  response.body = {sent: true}
}
```

## Live Dashboard

```httpdsl
server {
  port 3000
}

route SSE "/dashboard/events" {
  stream.send("init", {message: "Dashboard connected"})
}

every 5 s {
  stats = server_stats()
  
  broadcast("stats", {
    memory: stats.mem_alloc_mb,
    goroutines: stats.goroutines,
    uptime: stats.uptime_formatted
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
          const eventSource = new EventSource('/dashboard/events');
          
          eventSource.addEventListener('stats', (e) => {
            const data = JSON.parse(e.data);
            document.getElementById('stats').innerHTML = 
              '<p>Memory: ' + data.memory + ' MB</p>' +
              '<p>Goroutines: ' + data.goroutines + '</p>' +
              '<p>Uptime: ' + data.uptime + '</p>';
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
  user_id = session.user_id
  
  if !user_id {
    stream.send("error", {message: "Authentication required"})
    return
  }
  
  stream.join(`user:${user_id}`)
  
  stream.send("connected", {message: "Listening for notifications"})
}

route POST "/notify/:user_id" json {
  user_id = request.params.user_id
  message = request.data.message
  
  broadcast("notification", {
    message: message,
    timestamp: date()
  }, `user:${user_id}`)
  
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
  
  steps = 10
  
  each i in range(steps) {
    sleep(1000)
    
    progress = int((float(i + 1) / float(steps)) * 100)
    
    broadcast("progress", {
      job_id: job_id,
      step: i + 1,
      total: steps,
      progress: progress
    }, `job:${job_id}`)
  }
  
  broadcast("completed", {
    job_id: job_id,
    message: "Job completed"
  }, `job:${job_id}`)
  
  response.body = {completed: true}
}
```

## Log Streaming

```httpdsl
server {
  port 3000
}

route SSE "/logs" {
  stream.send("connected", {message: "Log stream started"})
  
  stream.join("logs")
}

before {
  if request.path != "/logs" {
    log_entry = {
      method: request.method,
      path: request.path,
      ip: request.ip,
      timestamp: date()
    }
    
    broadcast("log", log_entry, "logs")
  }
}

route GET "/" {
  response.body = "Home"
}

route GET "/api/data" {
  response.body = {data: "value"}
}
```

## Stock Ticker

```httpdsl
server {
  port 3000
}

route SSE "/ticker" {
  stream.send("connected", {message: "Stock ticker connected"})
}

every 2 s {
  price = 100 + rand(-10, 10)
  
  broadcast("price", {
    symbol: "ACME",
    price: price,
    timestamp: date()
  })
}

route GET "/" {
  response.type = "html"
  response.body = `
    <!DOCTYPE html>
    <html>
      <head><title>Stock Ticker</title></head>
      <body>
        <h1>ACME Stock Price</h1>
        <div id="price" style="font-size: 48px;">--</div>
        
        <script>
          const eventSource = new EventSource('/ticker');
          
          eventSource.addEventListener('price', (e) => {
            const data = JSON.parse(e.data);
            document.getElementById('price').textContent = 
              '$' + data.price.toFixed(2);
          });
        </script>
      </body>
    </html>
  `
}
```

## Multiplayer Game State

```httpdsl
server {
  port 3000
}

game_state = {
  players: []
}

route SSE "/game/:id/events" {
  game_id = request.params.id
  player_id = cuid2()
  
  stream.join(game_id)
  
  stream.send("joined", {
    player_id: player_id,
    game_id: game_id
  })
  
  broadcast("player_joined", {
    player_id: player_id
  }, game_id)
}

route POST "/game/:id/move" json {
  game_id = request.params.id
  {player_id, x, y} = request.data
  
  broadcast("player_moved", {
    player_id: player_id,
    x: x,
    y: y
  }, game_id)
  
  response.body = {moved: true}
}
```
