document.getElementById('new-game-btn').addEventListener('click', async function() {
  var btn = this;
  btn.disabled = true;
  btn.textContent = 'Creating...';
  try {
    var res = await fetch('/game/new', { method: 'POST' });
    var data = await res.json();
    if (data.key) {
      window.location.href = '/game/' + data.key;
    } else {
      document.getElementById('error-msg').textContent = data.error || 'Failed to create game';
      btn.disabled = false;
      btn.textContent = 'New Game';
    }
  } catch(e) {
    document.getElementById('error-msg').textContent = 'Connection error';
    btn.disabled = false;
    btn.textContent = 'New Game';
  }
});

document.getElementById('join-btn').addEventListener('click', function() {
  var key = document.getElementById('room-key').value.trim().toUpperCase();
  if (key.length !== 4) {
    document.getElementById('error-msg').textContent = 'Enter a 4-letter room key';
    return;
  }
  window.location.href = '/game/' + key;
});

document.getElementById('room-key').addEventListener('keydown', function(e) {
  if (e.key === 'Enter') document.getElementById('join-btn').click();
});
