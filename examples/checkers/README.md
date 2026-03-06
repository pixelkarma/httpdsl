# Checkers — HTTPDSL Example

A two-player checkers game with real-time updates via SSE.

## Features Used

| Feature | Usage |
|---------|-------|
| **SSE** | Real-time board sync between players |
| **Templates** | Go html/template for menu and game pages |
| **Static files** | CSS and JavaScript served from `./static` |
| **In-memory state** | Game state stored in a global hash |
| **Scheduled tasks** | 5-minute cleanup of abandoned games |
| **Functions** | Board setup, move validation, win detection |
| **CLI flags** | `-p` for port override |

## How It Works

1. Player 1 clicks **New Game** → gets a 4-letter room key
2. Player 2 enters the room key and clicks **Join**
3. Both players see the board; red moves first
4. Tap a piece to select it, tap a highlighted square to move
5. Jumps capture opponent pieces
6. Pieces reaching the back row become kings (marked with `*`)
7. Game ends when a player has no pieces or no valid moves
8. Both players get a rematch prompt

## Running

```bash
cd examples/checkers

# Build
../../httpdsl build app.httpdsl

# Run
./checkers
# → http://localhost:8080

# Custom port
./checkers -p 3000
```

Open two browser tabs to play against yourself.

## Game Rules

- Standard 8×8 board, 12 pieces per player
- Red moves first
- Regular pieces move diagonally forward one square
- Jumps capture the opponent’s piece (diagonal, two squares)
- No forced jumps, no multi-jumps
- Kings (back row) move and jump in all diagonal directions
- Win by capturing all opponent pieces or leaving them with no moves

## File Structure

```
examples/checkers/
├── app.httpdsl         # Server: routes, game logic, SSE
├── static/
│   ├── style.css       # Board and UI styles
│   ├── game.js         # Board rendering, move logic, SSE client
│   └── menu.js         # New game / join game handlers
└── templates/
    ├── menu.gohtml     # Main menu page
    └── game.gohtml     # Game board page
```
