/**
 * IFS-Kiseki — Sidebar Component
 *
 * Responsibilities:
 *   - Fetch and display past sessions in the sidebar
 *   - Handle session click to load messages
 *   - Highlight the active session
 *
 * Exposed as a global `Sidebar` object (vanilla JS, no module system).
 * app.js calls Sidebar.init() and Sidebar.refresh().
 *
 * API used:
 *   GET /api/sessions          → sessionRow[]
 *   GET /api/sessions/{id}     → sessionDetail (with messages[])
 */

'use strict';

const Sidebar = (() => {

  // ── Private state ──────────────────────────────────────────────

  let _onSessionLoad = null;       // callback(sessionDetail) when user clicks a session
  let _currentSessionId = null;    // highlighted session id

  // ── Public API ─────────────────────────────────────────────────

  /**
   * Initialise the sidebar component.
   * @param {{ onSessionLoad: function }} options
   */
  function init(options) {
    _onSessionLoad = (options && options.onSessionLoad) || null;
    refresh();
  }

  /**
   * Refresh the session list from the server.
   */
  async function refresh() {
    const container = document.getElementById('session-list');
    if (!container) return;

    try {
      const res = await fetch('/api/sessions');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const sessions = await res.json();

      container.innerHTML = '';

      if (sessions.length === 0) {
        const empty = document.createElement('p');
        empty.className = 'sidebar-empty';
        empty.textContent = 'No past sessions yet.';
        container.appendChild(empty);
        return;
      }

      sessions.forEach(session => {
        container.appendChild(_buildSessionItem(session));
      });
    } catch (err) {
      console.error('[sidebar] failed to load sessions:', err);
      container.innerHTML = '<p class="sidebar-empty">Could not load sessions.</p>';
    }
  }

  /**
   * Set the currently active session ID (for highlighting).
   * @param {string} sessionId
   */
  function setActive(sessionId) {
    _currentSessionId = sessionId;
    document.querySelectorAll('.session-item').forEach(el => {
      el.classList.toggle('active', el.dataset.id === sessionId);
    });
  }

  // ── Session item builder ───────────────────────────────────────

  function _buildSessionItem(session) {
    const btn = document.createElement('button');
    btn.className = 'session-item';
    btn.dataset.id = session.id;
    btn.setAttribute('role', 'listitem');

    if (session.id === _currentSessionId) {
      btn.classList.add('active');
    }

    const dateStr = _formatDate(session.started_at);
    const preview = session.summary
      ? _truncate(session.summary, 50)
      : dateStr;

    const dateSpan = document.createElement('span');
    dateSpan.className = 'session-item-date';
    dateSpan.textContent = dateStr;

    const previewSpan = document.createElement('span');
    previewSpan.className = 'session-item-preview';
    previewSpan.textContent = preview;

    btn.appendChild(dateSpan);
    btn.appendChild(previewSpan);

    btn.addEventListener('click', () => _handleClick(session.id));

    return btn;
  }

  async function _handleClick(sessionId) {
    // Highlight immediately.
    setActive(sessionId);

    try {
      const res = await fetch(`/api/sessions/${encodeURIComponent(sessionId)}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const detail = await res.json();

      if (typeof _onSessionLoad === 'function') {
        _onSessionLoad(detail);
      }
    } catch (err) {
      console.error('[sidebar] failed to load session:', err);
    }
  }

  // ── Formatting helpers ─────────────────────────────────────────

  function _formatDate(unixSec) {
    if (!unixSec) return '';
    const d = new Date(unixSec * 1000);
    return d.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
  }

  function _truncate(str, maxLen) {
    if (!str) return '';
    if (str.length <= maxLen) return str;
    return str.slice(0, maxLen).trimEnd() + '…';
  }

  // ── Expose public API ──────────────────────────────────────────

  return {
    init,
    refresh,
    setActive,
  };

})();
