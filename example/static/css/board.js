// ===== Utilities =====
function showToast(msg) {
  var c = document.getElementById('toast-container');
  var t = document.createElement('div');
  t.className = 'toast';
  t.textContent = msg;
  c.appendChild(t);
  setTimeout(function() {
    t.classList.add('removing');
    t.addEventListener('animationend', function() { t.remove(); });
  }, 3000);
}

function updateCounts() {
  document.querySelectorAll('.column').forEach(function(col) {
    var count = col.querySelectorAll('.task-card').length;
    var badge = col.querySelector('.count');
    if (badge) badge.textContent = count;
  });
}

function updateEmptyStates() {
  document.querySelectorAll('.column-body').forEach(function(body) {
    var cards = body.querySelectorAll('.task-card');
    var empty = body.querySelector('.empty-state');
    if (cards.length === 0 && !empty) {
      empty = document.createElement('div');
      empty.className = 'empty-state';
      empty.textContent = 'No tasks';
      body.appendChild(empty);
    } else if (cards.length > 0 && empty) {
      empty.remove();
    }
  });
}

function refreshBoard() {
  setTimeout(function() { updateCounts(); updateEmptyStates(); }, 10);
}

// ===== HTMX hooks =====
document.body.addEventListener('htmx:afterSwap', refreshBoard);
document.body.addEventListener('htmx:afterSettle', refreshBoard);

document.body.addEventListener('htmx:afterRequest', function(e) {
  if (e.detail.elt && e.detail.elt.id === 'new-task-form' && e.detail.successful) {
    e.detail.elt.querySelector('input[name=title]').value = '';
    e.detail.elt.querySelector('input[name=title]').focus();
  }
});

// Handle task move via HX-Trigger from PATCH response
document.body.addEventListener('moveTask', function(e) {
  var data = e.detail;
  var card = document.getElementById('task-' + data.id);
  if (card) card.remove();
  var col = document.getElementById('col-' + data.status);
  if (col) {
    var empty = col.querySelector('.empty-state');
    if (empty) empty.remove();
    col.insertAdjacentHTML('beforeend', data.html);
    var newCard = document.getElementById('task-' + data.id);
    if (newCard) htmx.process(newCard);
    refreshBoard();
  }
});

// ===== SSE: live updates from other clients =====
(function() {
  var dot = document.getElementById('conn-dot');
  var container = document.querySelector('[data-username]');
  var currentUser = container ? container.dataset.username : '';
  var es;

  function connect() {
    es = new EventSource('/events');

    es.onopen = function() {
      if (dot) { dot.classList.add('connected'); dot.title = 'Live connected'; }
    };

    es.onerror = function() {
      if (dot) { dot.classList.remove('connected'); dot.title = 'Disconnected \u2014 retrying...'; }
      // EventSource auto-reconnects, but update the UI
    };

    es.addEventListener('connected', function() {
      // Initial handshake confirmed
    });

    es.addEventListener('task-created', function(e) {
      var data = JSON.parse(e.data);
      if (data.user === currentUser) return; // own action — already handled by HTMX
      if (document.getElementById('task-' + data.id)) return;
      var col = document.getElementById('col-todo');
      if (col) {
        var empty = col.querySelector('.empty-state');
        if (empty) empty.remove();
        col.insertAdjacentHTML('beforeend', data.html);
        var newCard = document.getElementById('task-' + data.id);
        if (newCard) htmx.process(newCard);
        refreshBoard();
        showToast(data.user + ' added: ' + data.title);
      }
    });

    es.addEventListener('task-moved', function(e) {
      var data = JSON.parse(e.data);
      if (data.user === currentUser) return;
      var card = document.getElementById('task-' + data.id);
      if (card) card.remove();
      var col = document.getElementById('col-' + data.status);
      if (col) {
        var empty = col.querySelector('.empty-state');
        if (empty) empty.remove();
        col.insertAdjacentHTML('beforeend', data.html);
        var newCard = document.getElementById('task-' + data.id);
        if (newCard) htmx.process(newCard);
        refreshBoard();
        showToast(data.user + ' moved task to ' + data.status);
      }
    });

    es.addEventListener('task-deleted', function(e) {
      var data = JSON.parse(e.data);
      if (data.user === currentUser) return;
      var card = document.getElementById('task-' + data.id);
      if (card) {
        card.classList.add('anim-card-out');
        card.addEventListener('animationend', function() {
          card.remove();
          refreshBoard();
        });
        showToast(data.user + ' deleted a task');
      }
    });
  }

  // Only connect on the board page
  if (document.getElementById('col-todo')) {
    connect();
  }
})();
