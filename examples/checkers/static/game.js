(function() {
  var G = window.GAME;
  var boardEl = document.getElementById('board');
  var board = [];
  var selected = null;
  var validMoves = [];
  var myTurn = false;
  var gameActive = false;
  var mustJumpFrom = null;
  var es = null;

  var statusEl = document.getElementById('status');
  var gameOverEl = document.getElementById('game-over');
  var gameOverMsg = document.getElementById('game-over-msg');

  // Initialize empty board
  function initBoard(state) {
    board = state;
    selected = null;
    validMoves = [];
    renderBoard();
  }

  function renderBoard() {
    boardEl.innerHTML = '';
    // If player is red, board is shown as-is (red at bottom)
    // If player is black, flip the board (black at bottom)
    for (var vi = 0; vi < 8; vi++) {
      for (var vj = 0; vj < 8; vj++) {
        var r = G.color === 'black' ? (7 - vi) : vi;
        var c = G.color === 'black' ? (7 - vj) : vj;
        var sq = document.createElement('div');
        var isDark = (r + c) % 2 === 1;
        sq.className = 'square ' + (isDark ? 'dark' : 'light');
        sq.dataset.row = r;
        sq.dataset.col = c;

        var piece = board[r][c];
        if (piece !== '.') {
          var p = document.createElement('div');
          var color = (piece === 'r' || piece === 'R') ? 'red' : 'black';
          var isKing = (piece === 'R' || piece === 'B');
          p.className = 'piece ' + color;
          if (isKing) {
            var star = document.createElement('span');
            star.className = 'king-mark';
            star.textContent = '*';
            p.appendChild(star);
          }
          if (selected && selected[0] === r && selected[1] === c) {
            p.classList.add('selected');
          }
          sq.appendChild(p);
        }

        // Highlight valid moves
        var isValid = false;
        for (var m = 0; m < validMoves.length; m++) {
          if (validMoves[m][0] === r && validMoves[m][1] === c) {
            isValid = true;
            break;
          }
        }
        if (isValid) {
          var dot = document.createElement('div');
          dot.className = 'valid-dot';
          sq.appendChild(dot);
        }

        sq.addEventListener('click', onSquareClick);
        boardEl.appendChild(sq);
      }
    }
  }

  function onSquareClick(e) {
    if (!myTurn || !gameActive) return;
    var sq = e.currentTarget;
    var r = parseInt(sq.dataset.row);
    var c = parseInt(sq.dataset.col);
    var piece = board[r][c];
    var myPiece = G.color === 'red' ? 'r' : 'b';
    var myKing = myPiece.toUpperCase();

    // If clicking a valid move destination, make the move
    if (selected) {
      for (var m = 0; m < validMoves.length; m++) {
        if (validMoves[m][0] === r && validMoves[m][1] === c) {
          sendMove(selected[0], selected[1], r, c);
          selected = null;
          validMoves = [];
          renderBoard();
          return;
        }
      }
    }

    // If must continue jumping, don't allow selecting other pieces
    if (mustJumpFrom) return;

    // If clicking own piece, select it
    if (piece === myPiece || piece === myKing) {
      selected = [r, c];
      validMoves = getValidMoves(r, c);
      renderBoard();
    } else {
      selected = null;
      validMoves = [];
      renderBoard();
    }
  }

  function getJumpMoves(r, c) {
    var piece = board[r][c];
    var moves = [];
    var dirs = [];
    if (piece === 'r' || piece === 'R') dirs.push([-1, -1], [-1, 1]);
    if (piece === 'b' || piece === 'B') dirs.push([1, -1], [1, 1]);
    if (piece === 'R') dirs.push([1, -1], [1, 1]);
    if (piece === 'B') dirs.push([-1, -1], [-1, 1]);
    var enemy = (piece === 'r' || piece === 'R') ? 'b' : 'r';
    var enemyKing = enemy.toUpperCase();
    for (var d = 0; d < dirs.length; d++) {
      var dr = dirs[d][0], dc = dirs[d][1];
      var nr = r + dr, nc = c + dc;
      var jr = r + dr * 2, jc = c + dc * 2;
      if (nr >= 0 && nr < 8 && nc >= 0 && nc < 8 &&
          jr >= 0 && jr < 8 && jc >= 0 && jc < 8 &&
          (board[nr][nc] === enemy || board[nr][nc] === enemyKing) &&
          board[jr][jc] === '.') {
        moves.push([jr, jc]);
      }
    }
    return moves;
  }

  function getValidMoves(r, c) {
    var piece = board[r][c];
    var moves = [];
    var dirs = [];

    if (piece === 'r' || piece === 'R') {
      dirs.push([-1, -1], [-1, 1]); // red moves up
    }
    if (piece === 'b' || piece === 'B') {
      dirs.push([1, -1], [1, 1]); // black moves down
    }
    if (piece === 'R' || piece === 'B') {
      // Kings move both ways
      if (piece === 'R') dirs.push([1, -1], [1, 1]);
      if (piece === 'B') dirs.push([-1, -1], [-1, 1]);
    }

    var enemy = (piece === 'r' || piece === 'R') ? 'b' : 'r';
    var enemyKing = enemy.toUpperCase();

    for (var d = 0; d < dirs.length; d++) {
      var dr = dirs[d][0], dc = dirs[d][1];
      var nr = r + dr, nc = c + dc;
      // Simple move
      if (nr >= 0 && nr < 8 && nc >= 0 && nc < 8 && board[nr][nc] === '.') {
        moves.push([nr, nc]);
      }
      // Jump
      var jr = r + dr * 2, jc = c + dc * 2;
      if (nr >= 0 && nr < 8 && nc >= 0 && nc < 8 &&
          jr >= 0 && jr < 8 && jc >= 0 && jc < 8 &&
          (board[nr][nc] === enemy || board[nr][nc] === enemyKing) &&
          board[jr][jc] === '.') {
        moves.push([jr, jc]);
      }
    }
    return moves;
  }

  function sendMove(fromR, fromC, toR, toC) {
    fetch('/game/' + G.key + '/move', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ from: [fromR, fromC], to: [toR, toC] })
    });
  }

  function setStatus(text) {
    statusEl.textContent = text;
  }

  // SSE connection
  function connect() {
    if (es) es.close();
    es = new EventSource('/game/' + G.key + '/events?player=' + G.player);

    es.addEventListener('state', function(e) {
      var data = JSON.parse(e.data);
      board = data.board;
      gameActive = true;
      var turnColor = data.turn;
      myTurn = (turnColor === G.color);
      mustJumpFrom = data.must_jump_from || null;
      selected = null;
      validMoves = [];
      if (myTurn && mustJumpFrom) {
        // Auto-select the piece that must continue jumping
        selected = [mustJumpFrom[0], mustJumpFrom[1]];
        validMoves = getJumpMoves(selected[0], selected[1]);
        setStatus("Continue jumping!");
      } else if (myTurn) {
        setStatus("It's your turn");
      } else {
        setStatus("It's their turn");
      }
      renderBoard();
    });

    es.addEventListener('waiting', function(e) {
      setStatus('Waiting for opponent...');
      gameActive = false;
    });

    es.addEventListener('joined', function(e) {
      setStatus('Opponent joined!');
    });

    es.addEventListener('gameover', function(e) {
      var data = JSON.parse(e.data);
      gameActive = false;
      myTurn = false;
      board = data.board;
      renderBoard();
      var winner = data.winner;
      if (winner === G.color) {
        gameOverMsg.textContent = 'You won! Play again?';
      } else {
        gameOverMsg.textContent = 'You lost. Play again?';
      }
      gameOverEl.classList.remove('hidden');
    });

    es.addEventListener('rematch_waiting', function(e) {
      setStatus('Waiting for opponent to decide...');
    });

    es.addEventListener('rematch_declined', function(e) {
      window.location.href = '/';
    });

    es.addEventListener('opponent_disconnected', function(e) {
      setStatus('Opponent disconnected. Waiting for reconnect...');
    });

    es.addEventListener('opponent_reconnected', function(e) {
      setStatus('Opponent reconnected!');
    });

    es.addEventListener('error', function(e) {
      // EventSource auto-reconnects
    });
  }

  // Play again buttons
  document.getElementById('play-again-yes').addEventListener('click', function() {
    gameOverEl.classList.add('hidden');
    fetch('/game/' + G.key + '/rematch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ want: true })
    });
    setStatus('Waiting for opponent to decide...');
  });

  document.getElementById('play-again-no').addEventListener('click', function() {
    fetch('/game/' + G.key + '/rematch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ want: false })
    });
    window.location.href = '/';
  });

  // Render empty board immediately
  var emptyBoard = [];
  for (var r = 0; r < 8; r++) {
    var row = [];
    for (var c = 0; c < 8; c++) row.push('.');
    emptyBoard.push(row);
  }
  initBoard(emptyBoard);

  connect();
})();
