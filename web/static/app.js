/**
 * IFS-Kiseki — Main Application
 *
 * Responsibilities:
 *   - Onboarding gate (disclaimer + API key — via Onboarding module)
 *   - WebSocket lifecycle (connect, reconnect, dispatch incoming messages)
 *   - Global application state
 *   - Hash-based router (#chat, #settings)
 *   - UI wiring (nav buttons, sidebar toggle, new session button)
 *
 * Does NOT render chat messages — that lives in components/chat.js.
 * Does NOT render the onboarding overlay — that lives in components/onboarding.js.
 */

'use strict';

// ── Constants ────────────────────────────────────────────────────

const WS_URL = `ws://${location.host}/ws`;
const RECONNECT_DELAY_MS = 3000;
const MAX_RECONNECT_ATTEMPTS = 10;

// ── Application state ────────────────────────────────────────────

const state = {
  ws: null,                  // active WebSocket instance
  sessionId: null,           // current session ID (from server)
  connected: false,          // WebSocket connected flag
  streaming: false,          // true while assistant is streaming a response
  reconnectAttempts: 0,      // consecutive failed reconnect attempts
  reconnectTimer: null,      // pending reconnect setTimeout handle
};

// ── WebSocket management ─────────────────────────────────────────

/**
 * Open a WebSocket connection to the server.
 * Idempotent — if already connected, does nothing.
 */
function connectWebSocket() {
  if (state.ws && (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)) {
    return;
  }

  setConnectionStatus('connecting');

  const ws = new WebSocket(WS_URL);
  state.ws = ws;

  ws.addEventListener('open', () => {
    state.connected = true;
    state.reconnectAttempts = 0;
    setConnectionStatus('connected');
    console.log('[ws] connected');
  });

  ws.addEventListener('message', (event) => {
    let msg;
    try {
      msg = JSON.parse(event.data);
    } catch (err) {
      console.error('[ws] failed to parse message:', event.data, err);
      return;
    }
    handleServerMessage(msg);
  });

  ws.addEventListener('close', (event) => {
    state.connected = false;
    state.ws = null;
    setConnectionStatus('error');
    console.log(`[ws] closed (code=${event.code})`);

    // If streaming was in progress, finalize the bubble with an error note.
    if (state.streaming) {
      Chat.endStreaming(null);
      state.streaming = false;
    }

    scheduleReconnect();
  });

  ws.addEventListener('error', (err) => {
    console.error('[ws] error:', err);
    // close event will fire after error — reconnect handled there.
  });
}

/**
 * Schedule a reconnect attempt with exponential backoff (capped).
 */
function scheduleReconnect() {
  if (state.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    console.warn('[ws] max reconnect attempts reached — giving up');
    setConnectionStatus('error');
    return;
  }

  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer);
  }

  const delay = RECONNECT_DELAY_MS * Math.min(Math.pow(1.5, state.reconnectAttempts), 8);
  state.reconnectAttempts++;
  console.log(`[ws] reconnecting in ${Math.round(delay)}ms (attempt ${state.reconnectAttempts})`);

  state.reconnectTimer = setTimeout(() => {
    state.reconnectTimer = null;
    connectWebSocket();
  }, delay);
}

/**
 * Send a JSON message over the WebSocket.
 * Returns false if not connected.
 */
function sendMessage(payload) {
  if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
    console.warn('[ws] cannot send — not connected');
    return false;
  }
  state.ws.send(JSON.stringify(payload));
  return true;
}

// ── Incoming message dispatch ────────────────────────────────────

/**
 * Route a parsed server message to the appropriate handler.
 */
function handleServerMessage(msg) {
  switch (msg.type) {
    case 'session_created':
      state.sessionId = msg.session_id;
      console.log(`[app] session created: ${state.sessionId}`);
      // Refresh sidebar to show the new session.
      if (typeof Sidebar !== 'undefined') {
        Sidebar.setActive(state.sessionId);
        Sidebar.refresh();
      }
      break;

    case 'session_loaded':
      state.sessionId = msg.session_id;
      console.log(`[app] session loaded: ${state.sessionId}`);
      // Render the loaded session's messages.
      Chat.clearMessages();
      if (msg.messages && msg.messages.length > 0) {
        msg.messages.forEach(function(m) {
          Chat.addMessage(m.role, m.content);
        });
      }
      Chat.setInputEnabled(true);
      if (typeof Sidebar !== 'undefined') {
        Sidebar.setActive(state.sessionId);
      }
      break;

    case 'delta':
      // First delta — replace thinking indicator with streaming bubble.
      if (!state.streaming) {
        state.streaming = true;
        Chat.startStreaming();
      }
      Chat.appendDelta(msg.content || '');
      break;

    case 'done':
      state.streaming = false;
      Chat.endStreaming(msg.usage || null);
      Chat.setInputEnabled(true);
      break;

    case 'crisis':
      // Crisis content detected — show the overlay with resources.
      // The message was NOT sent to the LLM (server intercepted it).
      // Re-enable input so the user can continue after dismissing the overlay.
      state.streaming = false;
      Chat.removeThinking();
      Chat.setInputEnabled(true);
      CrisisOverlay.show(msg.resources || '');
      break;

    case 'error':
      state.streaming = false;
      Chat.removeThinking();
      Chat.showError(msg.message || 'An error occurred.');
      Chat.setInputEnabled(true);
      break;

    default:
      console.warn('[ws] unknown message type:', msg.type);
  }
}

// ── Public API used by chat component ────────────────────────────

/**
 * Send a user chat message.
 * Called by the chat component's submit handler.
 */
function sendChatMessage(content) {
  if (!content || !content.trim()) return;
  if (state.streaming) return;

  const ok = sendMessage({ type: 'message', content: content.trim() });
  if (!ok) {
    Chat.showError('Not connected to server. Reconnecting…');
    return;
  }

  Chat.addMessage('user', content.trim());
  Chat.showThinking();
  Chat.setInputEnabled(false);
}

/**
 * Request a new session from the server.
 */
function requestNewSession() {
  if (state.streaming) return;
  sendMessage({ type: 'new_session' });
  Chat.clearMessages();
}

// ── Connection status UI ─────────────────────────────────────────

/**
 * Update the connection status dot and its aria-label.
 * @param {'connecting'|'connected'|'error'|'idle'} status
 */
function setConnectionStatus(status) {
  const dot = document.getElementById('connection-status');
  if (!dot) return;

  dot.className = `status-dot status-${status}`;

  const labels = {
    connecting: 'Connection status: connecting',
    connected:  'Connection status: connected',
    error:      'Connection status: disconnected',
    idle:       'Connection status: idle',
  };
  dot.setAttribute('aria-label', labels[status] || 'Connection status: unknown');
}

// ── Router ───────────────────────────────────────────────────────

/**
 * Navigate to a named view. Settings is now a slide-in panel,
 * not a separate view — use toggleSettingsPanel() for that.
 * @param {'chat'|'settings'} route
 */
function navigate(route) {
  // If navigating to settings, open the panel instead of swapping views.
  if (route === 'settings') {
    openSettingsPanel();
    return;
  }

  // Default to chat for any unknown route.
  route = 'chat';

  // Update hash without triggering hashchange loop.
  if (location.hash !== `#${route}`) {
    history.replaceState(null, '', `#${route}`);
  }

  // Ensure chat view is visible (it should always be visible now).
  const chatView = document.getElementById('chat-container');
  if (chatView) {
    chatView.hidden = false;
    chatView.classList.add('view-active');
  }

  // Update nav button active state — chat is always the active page.
  const chatBtn = document.querySelector('.nav-btn[data-route="chat"]');
  if (chatBtn) {
    chatBtn.classList.add('nav-btn-active');
    chatBtn.setAttribute('aria-current', 'page');
  }

  // Close settings panel if open when navigating to chat.
  closeSettingsPanel();
}

// ── Settings panel ───────────────────────────────────────────────

/**
 * Toggle the settings panel open/closed.
 * When closing, auto-saves settings.
 */
function toggleSettingsPanel() {
  const panel = document.getElementById('settings-panel');
  if (!panel) return;

  const isOpen = panel.classList.contains('settings-panel--open');
  if (isOpen) {
    closeSettingsPanel();
  } else {
    openSettingsPanel();
  }
}

/**
 * Open the settings panel. Lazy-renders settings on first open.
 */
function openSettingsPanel() {
  const panel = document.getElementById('settings-panel');
  if (!panel) return;

  // Already open — do nothing.
  if (panel.classList.contains('settings-panel--open')) return;

  // Show the panel element, then trigger the slide-in on next frame.
  panel.hidden = false;
  panel.removeAttribute('hidden');

  // Force a reflow so the CSS transition plays from the collapsed state.
  void panel.offsetHeight;

  panel.classList.add('settings-panel--open');

  // Update hash.
  if (location.hash !== '#settings') {
    history.replaceState(null, '', '#settings');
  }

  // Highlight the settings nav button.
  const settingsBtn = document.querySelector('.nav-btn[data-route="settings"]');
  if (settingsBtn) {
    settingsBtn.classList.add('nav-btn-active');
    settingsBtn.setAttribute('aria-current', 'true');
  }

  // Lazy-render settings content on first open.
  if (typeof Settings !== 'undefined') {
    Settings.render();
  }
}

/**
 * Close the settings panel and auto-save settings.
 */
function closeSettingsPanel() {
  const panel = document.getElementById('settings-panel');
  if (!panel) return;

  // Not open — do nothing.
  if (!panel.classList.contains('settings-panel--open')) return;

  // Auto-save before closing.
  if (typeof Settings !== 'undefined' && typeof Settings.save === 'function') {
    Settings.save();
  }

  // Slide out.
  panel.classList.remove('settings-panel--open');

  // After transition ends, hide the element from the DOM flow.
  function onTransitionEnd(e) {
    // Only react to the transform transition on the panel itself.
    if (e.target !== panel) return;
    panel.removeEventListener('transitionend', onTransitionEnd);
    if (!panel.classList.contains('settings-panel--open')) {
      panel.hidden = true;
    }
  }
  panel.addEventListener('transitionend', onTransitionEnd);

  // Remove settings nav button highlight.
  const settingsBtn = document.querySelector('.nav-btn[data-route="settings"]');
  if (settingsBtn) {
    settingsBtn.classList.remove('nav-btn-active');
    settingsBtn.setAttribute('aria-current', 'false');
  }

  // Update hash back to chat.
  if (location.hash === '#settings') {
    history.replaceState(null, '', '#chat');
  }
}

// ── Sidebar toggle ───────────────────────────────────────────────

function toggleSidebar() {
  const sidebar = document.getElementById('sidebar');
  const btn = document.getElementById('sidebar-toggle');
  if (!sidebar || !btn) return;

  const isCollapsed = sidebar.classList.toggle('collapsed');
  btn.setAttribute('aria-expanded', String(!isCollapsed));
}

// ── Initialisation ───────────────────────────────────────────────

/**
 * Bootstrap the application. Called once on DOMContentLoaded.
 *
 * Onboarding gate: if the disclaimer has not been accepted, the Onboarding
 * module shows its overlay and calls _startApp() when the user completes or
 * skips the flow. This ensures the user sees the disclaimer before the chat.
 */
function init() {
  // Apply saved theme and font size from server settings.
  applySettingsFromServer();

  // Check disclaimer status. If not accepted, show onboarding overlay.
  // _startApp() is called as the completion callback — it runs after the
  // user finishes (or skips) onboarding.
  if (typeof Onboarding !== 'undefined') {
    Onboarding.init(_startApp);
  } else {
    // Onboarding module not loaded — proceed directly (should not happen).
    console.warn('[app] Onboarding module not found — skipping onboarding check');
    _startApp();
  }
}

/**
 * Start the main application after onboarding is complete (or skipped).
 * Wires all UI components and opens the WebSocket connection.
 */
function _startApp() {
  // Initialise the chat component (renders the message list + input form).
  Chat.init({
    onSend: sendChatMessage,
  });

  // Initialise the sidebar component (session list).
  if (typeof Sidebar !== 'undefined') {
    Sidebar.init({
      onSessionLoad: function(detail) {
        // Load a past session's messages into the chat view.
        Chat.clearMessages();
        if (detail && detail.messages) {
          detail.messages.forEach(function(msg) {
            Chat.addMessage(msg.role, msg.content);
          });
        }
        navigate('chat');
      },
      onNewSession: requestNewSession,
    });
  }

  // Wire nav buttons.
  document.querySelectorAll('.nav-btn').forEach((btn) => {
    if (btn.dataset.route === 'settings') {
      // Settings button toggles the panel instead of navigating.
      btn.addEventListener('click', toggleSettingsPanel);
    } else {
      btn.addEventListener('click', () => navigate(btn.dataset.route));
    }
  });

  // Wire settings panel close button.
  const settingsCloseBtn = document.getElementById('settings-panel-close');
  if (settingsCloseBtn) {
    settingsCloseBtn.addEventListener('click', closeSettingsPanel);
  }

  // Wire sidebar toggle.
  const sidebarToggle = document.getElementById('sidebar-toggle');
  if (sidebarToggle) {
    sidebarToggle.addEventListener('click', toggleSidebar);
  }

  // Wire new session button.
  const newSessionBtn = document.getElementById('new-session-btn');
  if (newSessionBtn) {
    newSessionBtn.addEventListener('click', requestNewSession);
  }

  // Handle hash-based routing on load and on back/forward.
  window.addEventListener('hashchange', () => {
    const route = location.hash.replace('#', '') || 'chat';
    navigate(route);
  });

  // Navigate to initial route.
  const initialRoute = location.hash.replace('#', '') || 'chat';
  navigate(initialRoute);

  // Connect WebSocket.
  connectWebSocket();
}

/**
 * Fetch settings from the server and apply theme/font-size to the document.
 * Fails silently — defaults are already set in CSS.
 */
async function applySettingsFromServer() {
  try {
    const res = await fetch('/api/settings');
    if (!res.ok) return;
    const cfg = await res.json();

    if (cfg.ui) {
      if (cfg.ui.theme) {
        document.documentElement.setAttribute('data-theme', cfg.ui.theme);
      }
      if (cfg.ui.font_size) {
        document.documentElement.setAttribute('data-font-size', cfg.ui.font_size);
      }
    }
  } catch (_) {
    // Server not ready yet — CSS defaults apply.
  }
}

// ── Boot ─────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', init);
