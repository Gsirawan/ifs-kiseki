/**
 * IFS-Kiseki — Sidebar Component
 *
 * Responsibilities:
 *   - Fetch and display past sessions in the sidebar
 *   - Handle session click to load messages
 *   - Highlight the active session
 *   - Delete sessions with confirmation
 *
 * Exposed as a global `Sidebar` object (vanilla JS, no module system).
 * app.js calls Sidebar.init() and Sidebar.refresh().
 *
 * API used:
 *   GET /api/sessions          → sessionRow[]
 *   GET /api/sessions/{id}     → sessionDetail (with messages[])
 *   DELETE /api/sessions/{id}  → {"status":"deleted"}
 */

'use strict';

const Sidebar = (() => {

  // ── Private state ──────────────────────────────────────────────

  let _onSessionLoad = null;       // callback(sessionDetail) when user clicks a session
  let _onNewSession = null;        // callback() to start a new session after deleting the active one
  let _currentSessionId = null;    // highlighted session id
  let _confirmOverlay = null;      // active delete-confirmation overlay (null when not shown)

  // ── Public API ─────────────────────────────────────────────────

  /**
   * Initialise the sidebar component.
   * @param {{ onSessionLoad: function }} options
   */
  function init(options) {
    _onSessionLoad = (options && options.onSessionLoad) || null;
    _onNewSession = (options && options.onNewSession) || null;
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

    // Top row: date + duration + delete button
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

    // Delete button — visible on hover only (CSS handles visibility).
    const deleteBtn = document.createElement('span');
    deleteBtn.className = 'session-item-delete';
    deleteBtn.setAttribute('role', 'button');
    deleteBtn.setAttribute('aria-label', 'Delete session');
    deleteBtn.setAttribute('tabindex', '0');
    deleteBtn.textContent = '×';
    deleteBtn.addEventListener('click', (e) => {
      e.stopPropagation(); // don't trigger session load
      _showDeleteConfirm(session.id);
    });
    deleteBtn.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        e.stopPropagation();
        _showDeleteConfirm(session.id);
      }
    });
    metaRow.appendChild(deleteBtn);

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

  // ── Delete confirmation ─────────────────────────────────────────

  /**
   * Show a styled confirmation modal before deleting a session.
   * This is a therapy app — accidental deletion of session history would be harmful.
   * @param {string} sessionId
   */
  function _showDeleteConfirm(sessionId) {
    // Prevent stacking multiple confirm dialogs.
    _dismissDeleteConfirm();

    const overlay = document.createElement('div');
    overlay.className = 'delete-confirm-overlay';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-labelledby', 'delete-confirm-title');

    overlay.innerHTML = `
      <div class="delete-confirm-backdrop" aria-hidden="true"></div>
      <div class="delete-confirm-panel">
        <p id="delete-confirm-title" class="delete-confirm-title">Delete this session?</p>
        <p class="delete-confirm-message">This cannot be undone. All messages in this session will be permanently removed.</p>
        <div class="delete-confirm-actions">
          <button class="delete-confirm-btn delete-confirm-btn--cancel" data-action="cancel">Cancel</button>
          <button class="delete-confirm-btn delete-confirm-btn--delete" data-action="delete">Delete</button>
        </div>
      </div>
    `;

    // Wire button handlers.
    overlay.querySelector('[data-action="cancel"]').addEventListener('click', () => {
      _dismissDeleteConfirm();
    });
    overlay.querySelector('[data-action="delete"]').addEventListener('click', () => {
      _dismissDeleteConfirm();
      _executeDelete(sessionId);
    });

    // Close on backdrop click.
    overlay.querySelector('.delete-confirm-backdrop').addEventListener('click', () => {
      _dismissDeleteConfirm();
    });

    // Close on Escape key.
    function handleEscape(e) {
      if (e.key === 'Escape') {
        _dismissDeleteConfirm();
        document.removeEventListener('keydown', handleEscape);
      }
    }
    document.addEventListener('keydown', handleEscape);

    // Store reference for cleanup and attach the escape handler to the overlay
    // so _dismissDeleteConfirm can remove it.
    overlay._escapeHandler = handleEscape;

    document.body.appendChild(overlay);
    _confirmOverlay = overlay;

    // Focus the cancel button (safe default — don't make delete the easy path).
    overlay.querySelector('[data-action="cancel"]').focus();
  }

  /**
   * Remove the delete confirmation overlay if present.
   */
  function _dismissDeleteConfirm() {
    if (_confirmOverlay) {
      if (_confirmOverlay._escapeHandler) {
        document.removeEventListener('keydown', _confirmOverlay._escapeHandler);
      }
      _confirmOverlay.remove();
      _confirmOverlay = null;
    }
  }

  /**
   * Execute the session deletion via the REST API.
   * On success: remove the item from the list and handle active-session cleanup.
   * On failure: show a brief error message.
   * @param {string} sessionId
   */
  async function _executeDelete(sessionId) {
    try {
      const res = await fetch(`/api/sessions/${encodeURIComponent(sessionId)}`, {
        method: 'DELETE',
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || `HTTP ${res.status}`);
      }

      // If the deleted session was the currently active one, start a new session.
      const wasActive = (sessionId === _currentSessionId);
      if (wasActive) {
        _currentSessionId = null;
        if (typeof _onNewSession === 'function') {
          _onNewSession();
        } else if (typeof requestNewSession === 'function') {
          // Fallback to the global function from app.js.
          requestNewSession();
        }
      }

      // Refresh the session list to reflect the deletion.
      refresh();
    } catch (err) {
      console.error('[sidebar] failed to delete session:', err);
      _showDeleteError(err.message || 'Could not delete session.');
    }
  }

  /**
   * Show a brief, auto-dismissing error toast when deletion fails.
   * @param {string} message
   */
  function _showDeleteError(message) {
    // Remove any existing error toast.
    const existing = document.getElementById('sidebar-delete-error');
    if (existing) existing.remove();

    const toast = document.createElement('div');
    toast.id = 'sidebar-delete-error';
    toast.className = 'delete-error-toast';
    toast.setAttribute('role', 'alert');
    toast.textContent = message;

    document.body.appendChild(toast);

    // Auto-dismiss after 4 seconds.
    setTimeout(() => {
      toast.classList.add('delete-error-toast--fading');
      setTimeout(() => toast.remove(), 300);
    }, 4000);
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
