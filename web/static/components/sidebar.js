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
    _fetchBriefing();
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

  // ── Briefing card ─────────────────────────────────────────────

  /**
   * Fetch the session briefing and display it at the top of the sidebar.
   * Gracefully degrades: shows nothing on error or empty briefing.
   */
  async function _fetchBriefing() {
    const container = document.getElementById('sidebar-briefing');
    if (!container) return;

    try {
      const res = await fetch('/api/briefing');
      if (!res.ok) {
        // 503 = no provider configured — show nothing, not an error.
        container.hidden = true;
        return;
      }
      const data = await res.json();
      const text = (data.briefing || '').trim();

      if (!text) {
        container.hidden = true;
        return;
      }

      container.textContent = text;
      container.hidden = false;
    } catch (err) {
      // Network error or unexpected failure — hide gracefully.
      console.error('[sidebar] briefing fetch failed:', err);
      container.hidden = true;
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
    const duration = _formatDuration(session.started_at, session.ended_at);
    const preview = session.summary
      ? _truncate(session.summary, 50)
      : dateStr;

    // Top row: date + duration
    const metaRow = document.createElement('span');
    metaRow.className = 'session-item-meta';

    const dateSpan = document.createElement('span');
    dateSpan.className = 'session-item-date';
    dateSpan.textContent = dateStr;
    metaRow.appendChild(dateSpan);

    if (duration) {
      const durSpan = document.createElement('span');
      durSpan.className = 'session-item-duration';
      durSpan.textContent = duration;
      metaRow.appendChild(durSpan);
    }

    const previewSpan = document.createElement('span');
    previewSpan.className = 'session-item-preview';
    previewSpan.textContent = preview;

    btn.appendChild(metaRow);
    btn.appendChild(previewSpan);

    btn.addEventListener('click', () => _handleClick(session.id));

    return btn;
  }

  async function _handleClick(sessionId) {
    // Highlight immediately.
    setActive(sessionId);

    // Use WebSocket switch_session if available — this both loads the session
    // for display AND makes it the active session for new messages.
    if (typeof sendMessage === 'function' && typeof state !== 'undefined' && state.connected) {
      const sent = sendMessage({ type: 'switch_session', session_id: sessionId });
      if (sent) return; // session_loaded response handled by app.js
    }

    // Fallback to REST (read-only) if WebSocket is not connected.
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

  /**
   * Format session duration from started_at and ended_at unix timestamps.
   * Returns a human-readable string like "12 min", "1 hr 23 min", "< 1 min".
   * Returns "ongoing" if ended_at is null/0, or '' if started_at is missing.
   */
  function _formatDuration(startedAt, endedAt) {
    if (!startedAt) return '';
    if (!endedAt) return 'ongoing';

    const totalSec = endedAt - startedAt;
    if (totalSec < 0) return '';
    if (totalSec < 60) return '< 1 min';

    const hours = Math.floor(totalSec / 3600);
    const mins = Math.floor((totalSec % 3600) / 60);

    if (hours === 0) return mins + ' min';
    if (mins === 0) return hours + ' hr';
    return hours + ' hr ' + mins + ' min';
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
