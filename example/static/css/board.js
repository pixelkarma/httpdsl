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
  var es = null;
  var retryDelay = 1000; // start at 1s
  var maxRetryDelay = 30000; // cap at 30s
  var retryTimer = null;

  function setConnected(connected) {
    if (dot) {
      if (connected) {
        dot.classList.add('connected');
        dot.title = 'Live connected';
      } else {
        dot.classList.remove('connected');
        dot.title = 'Disconnected \u2014 retrying...';
      }
    }
  }

  function connect() {
    // Clean up previous connection
    if (es) {
      es.close();
      es = null;
    }

    console.log('[SSE] connecting to /events...');
    es = new EventSource('/events');

    es.onopen = function() {
      console.log('[SSE] connected');
      retryDelay = 1000; // reset backoff on success
      setConnected(true);
    };

    es.onerror = function() {
      // readyState 2 = CLOSED — browser gave up, we must reconnect manually
      // readyState 0 = CONNECTING — browser is auto-retrying (but we don't trust it)
      var wasClosed = es.readyState === 2;
      console.log('[SSE] ' + (wasClosed ? 'disconnected' : 'connection error') + ', retrying in ' + (retryDelay / 1000) + 's...');
      setConnected(false);

      // Always take control of reconnection
      es.close();
      es = null;

      if (retryTimer) clearTimeout(retryTimer);
      retryTimer = setTimeout(function() {
        retryTimer = null;
        connect();
      }, retryDelay);

      // Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s, 30s, ...
      retryDelay = Math.min(retryDelay * 2, maxRetryDelay);
    };

    es.addEventListener('connected', function() {
      // Server confirmed the SSE handshake
    });

    es.addEventListener('task-created', function(e) {
      var data = JSON.parse(e.data);
      if (data.user === currentUser) return;
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
