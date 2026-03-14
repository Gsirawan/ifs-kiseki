/**
 * IFS-Kiseki — Crisis Overlay Component
 *
 * Displayed when the server detects crisis content in a user message.
 *
 * Design principles (therapy product):
 *   - Warm amber/gold tones — NOT alarming red
 *   - Full-screen with soft background blur
 *   - Non-dismissable for 5 seconds (countdown timer)
 *   - Compassionate language — a hand reaching out, not a clinical warning
 *   - Phone numbers are clickable tel: links on mobile
 *   - Accessible: focus trap, screen reader friendly, role="dialog"
 *
 * Exposed as a global `CrisisOverlay` object (vanilla JS, no module system).
 * app.js calls CrisisOverlay.show(resources) when a crisis message arrives.
 */

'use strict';

const CrisisOverlay = (() => {

  // ── Constants ──────────────────────────────────────────────────

  const LOCK_SECONDS = 5;   // seconds before close button appears

  // ── Private state ──────────────────────────────────────────────

  let _overlay = null;       // the overlay DOM element (null when not shown)
  let _countdownTimer = null; // setInterval handle for the countdown
  let _previousFocus = null; // element focused before overlay opened

  // ── Phone number linkification ────────────────────────────────

  /**
   * Convert phone number patterns in text to clickable tel: links.
   * Handles formats: 988, 741741, 911, 999, 000, 1-800-xxx-xxxx, etc.
   * Only linkifies sequences that look like dialable numbers.
   *
   * Security: input is plain text (from server resources string),
   * not user-supplied HTML. We escape HTML before linkifying.
   */
  function linkifyResources(text) {
    // 1. Escape HTML entities first.
    let safe = text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');

    // 2. Linkify phone numbers.
    //    Pattern: optional country code, then digits/dashes/spaces/parens,
    //    at least 3 digits total. We match common formats.
    safe = safe.replace(
      /\b(\+?[\d][\d\s\-().]{2,}[\d])\b/g,
      (match) => {
        // Strip non-digit characters for the tel: href.
        const digits = match.replace(/\D/g, '');
        // Only linkify if it looks like a real number (3+ digits).
        if (digits.length < 3) return match;
        return `<a href="tel:${digits}" class="crisis-phone-link">${match}</a>`;
      }
    );

    // 3. Linkify URLs (the IASP link).
    safe = safe.replace(
      /(https?:\/\/[^\s<>"]+)/g,
      (url) => `<a href="${url}" target="_blank" rel="noopener noreferrer" class="crisis-url-link">${url}</a>`
    );

    // 4. Convert newlines to <br> for display.
    safe = safe.replace(/\n/g, '<br>');

    return safe;
  }

  // ── Overlay DOM builder ────────────────────────────────────────

  /**
   * Build and inject the overlay DOM into document.body.
   * @param {string} resources  Plain text resource string from server.
   */
  function buildOverlay(resources) {
    // Remove any stale overlay first.
    removeOverlay();

    const overlay = document.createElement('div');
    overlay.id = 'crisis-overlay';
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-labelledby', 'crisis-title');
    overlay.setAttribute('aria-describedby', 'crisis-body');

    overlay.innerHTML = `
      <div class="crisis-backdrop" aria-hidden="true"></div>
      <div class="crisis-panel" role="document">

        <div class="crisis-icon" aria-hidden="true">&#10084;&#65039;</div>

        <h2 id="crisis-title" class="crisis-title">
          You matter. Help is here.
        </h2>

        <p class="crisis-message">
          It sounds like you may be going through something very difficult right now.
          You don't have to face this alone — trained counselors are available
          right now, 24 hours a day.
        </p>

        <div id="crisis-body" class="crisis-resources" aria-label="Crisis resources">
          ${linkifyResources(resources)}
        </div>

        <div class="crisis-footer">
          <div id="crisis-countdown" class="crisis-countdown" aria-live="polite" aria-atomic="true">
            <!-- Countdown text injected by JS -->
          </div>
          <button
            id="crisis-close-btn"
            class="crisis-close-btn"
            aria-label="Close crisis resources"
            hidden
          >
            I've noted these resources
          </button>
        </div>

      </div>
    `;

    document.body.appendChild(overlay);
    _overlay = overlay;

    return overlay;
  }

  // ── Countdown logic ────────────────────────────────────────────

  /**
   * Start the 5-second countdown. After it expires, show the close button.
   */
  function startCountdown() {
    const countdownEl = document.getElementById('crisis-countdown');
    const closeBtn = document.getElementById('crisis-close-btn');
    if (!countdownEl || !closeBtn) return;

    let remaining = LOCK_SECONDS;

    function tick() {
      if (remaining > 0) {
        countdownEl.textContent = `Please take a moment — you can close this in ${remaining}s`;
        remaining--;
      } else {
        // Time's up — show close button, hide countdown.
        clearInterval(_countdownTimer);
        _countdownTimer = null;
        countdownEl.textContent = '';
        countdownEl.hidden = true;
        closeBtn.hidden = false;
        closeBtn.focus();
      }
    }

    tick(); // run immediately so there's no blank flash
    _countdownTimer = setInterval(tick, 1000);
  }

  // ── Focus trap ─────────────────────────────────────────────────

  /**
   * Trap keyboard focus inside the overlay while it is open.
   * Allows Tab and Shift+Tab to cycle through focusable elements.
   * Blocks Escape key while countdown is active.
   */
  function handleKeydown(e) {
    if (!_overlay) return;

    const focusable = _overlay.querySelectorAll(
      'a[href], button:not([hidden]):not([disabled]), [tabindex]:not([tabindex="-1"])'
    );
    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    if (e.key === 'Tab') {
      if (focusable.length === 0) {
        e.preventDefault();
        return;
      }
      if (e.shiftKey) {
        if (document.activeElement === first) {
          e.preventDefault();
          last.focus();
        }
      } else {
        if (document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }

    // Block Escape while countdown is running — overlay is non-dismissable.
    if (e.key === 'Escape') {
      const closeBtn = document.getElementById('crisis-close-btn');
      if (closeBtn && closeBtn.hidden) {
        e.preventDefault(); // still counting down — block dismiss
      } else {
        hide(); // countdown done — allow Escape to close
      }
    }
  }

  // ── Remove overlay ─────────────────────────────────────────────

  function removeOverlay() {
    if (_countdownTimer) {
      clearInterval(_countdownTimer);
      _countdownTimer = null;
    }
    document.removeEventListener('keydown', handleKeydown);

    if (_overlay) {
      _overlay.remove();
      _overlay = null;
    }
  }

  // ── Public API ─────────────────────────────────────────────────

  /**
   * Show the crisis overlay with the given resource text.
   * Non-dismissable for LOCK_SECONDS seconds.
   * @param {string} resources  Plain text from server {"type":"crisis","resources":"..."}
   */
  function show(resources) {
    // Save the currently focused element so we can restore it on close.
    _previousFocus = document.activeElement;

    const overlay = buildOverlay(resources || defaultResources());

    // Wire close button.
    const closeBtn = document.getElementById('crisis-close-btn');
    if (closeBtn) {
      closeBtn.addEventListener('click', hide);
    }

    // Trap focus inside overlay.
    document.addEventListener('keydown', handleKeydown);

    // Start countdown.
    startCountdown();

    // Move focus into the overlay panel for screen readers.
    const panel = overlay.querySelector('.crisis-panel');
    if (panel) {
      panel.setAttribute('tabindex', '-1');
      panel.focus();
    }

    // Announce to screen readers.
    overlay.setAttribute('aria-live', 'assertive');
  }

  /**
   * Hide and destroy the crisis overlay.
   * Only callable after the countdown has expired.
   */
  function hide() {
    removeOverlay();

    // Restore focus to where it was before the overlay opened.
    if (_previousFocus && typeof _previousFocus.focus === 'function') {
      _previousFocus.focus();
    }
    _previousFocus = null;
  }

  /**
   * Fallback resources if the server sends an empty string.
   * Should never happen in practice — server always sends resources.
   */
  function defaultResources() {
    return [
      'If you\'re in crisis, please reach out:',
      '',
      '988 Suicide & Crisis Lifeline: Call or text 988 (US)',
      'Crisis Text Line: Text HOME to 741741',
      'Emergency Services: Call 911',
      '',
      'Find a crisis centre near you:',
      'https://www.iasp.info/resources/Crisis_Centres/',
      '',
      'You are not alone. These feelings can change.',
      'A trained counselor is available 24/7.',
    ].join('\n');
  }

  return { show, hide };

})();

// ── Styles (injected into <head> — no external CSS file needed) ──

(function injectCrisisStyles() {
  if (document.getElementById('crisis-styles')) return; // idempotent

  const style = document.createElement('style');
  style.id = 'crisis-styles';
  style.textContent = `
    /* ── Crisis overlay — warm amber/gold, NOT alarming red ── */

    #crisis-overlay {
      position: fixed;
      inset: 0;
      z-index: 9999;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1rem;
      /* Prevent interaction with content behind overlay */
      pointer-events: all;
    }

    .crisis-backdrop {
      position: absolute;
      inset: 0;
      background: rgba(30, 20, 10, 0.72);
      backdrop-filter: blur(6px);
      -webkit-backdrop-filter: blur(6px);
    }

    .crisis-panel {
      position: relative;
      z-index: 1;
      background: #fffbf2;
      border: 2px solid #d4a843;
      border-radius: 16px;
      padding: 2rem 2.5rem;
      max-width: 560px;
      width: 100%;
      max-height: 90vh;
      overflow-y: auto;
      box-shadow:
        0 0 0 4px rgba(212, 168, 67, 0.18),
        0 20px 60px rgba(30, 20, 10, 0.35);
      animation: crisisFadeIn 300ms ease both;
    }

    @keyframes crisisFadeIn {
      from { opacity: 0; transform: scale(0.96) translateY(8px); }
      to   { opacity: 1; transform: scale(1)    translateY(0);   }
    }

    .crisis-icon {
      font-size: 2.5rem;
      text-align: center;
      margin-bottom: 0.75rem;
      line-height: 1;
    }

    .crisis-title {
      font-size: 1.5rem;
      font-weight: 700;
      color: #7a4f10;
      text-align: center;
      margin-bottom: 0.75rem;
      line-height: 1.3;
    }

    .crisis-message {
      font-size: 1rem;
      color: #5a3e1b;
      text-align: center;
      line-height: 1.65;
      margin-bottom: 1.5rem;
    }

    .crisis-resources {
      background: #fef3d0;
      border: 1px solid #e8c96a;
      border-radius: 10px;
      padding: 1.25rem 1.5rem;
      font-size: 0.9375rem;
      color: #3d2a08;
      line-height: 1.8;
      margin-bottom: 1.5rem;
    }

    .crisis-phone-link {
      color: #7a4f10;
      font-weight: 700;
      font-size: 1.0625rem;
      text-decoration: none;
      border-bottom: 2px solid #d4a843;
      padding-bottom: 1px;
      transition: color 150ms ease, border-color 150ms ease;
    }

    .crisis-phone-link:hover,
    .crisis-phone-link:focus {
      color: #5a3200;
      border-color: #5a3200;
      outline: none;
    }

    .crisis-url-link {
      color: #7a4f10;
      word-break: break-all;
      font-size: 0.875rem;
    }

    .crisis-url-link:hover {
      color: #5a3200;
    }

    .crisis-footer {
      text-align: center;
    }

    .crisis-countdown {
      font-size: 0.875rem;
      color: #9a7030;
      margin-bottom: 0.75rem;
      min-height: 1.4em;
      font-style: italic;
    }

    .crisis-close-btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      padding: 0.625rem 1.5rem;
      background: #d4a843;
      color: #3d2a08;
      border: none;
      border-radius: 8px;
      font-family: inherit;
      font-size: 0.9375rem;
      font-weight: 600;
      cursor: pointer;
      transition: background 150ms ease, transform 150ms ease;
    }

    .crisis-close-btn:hover {
      background: #c49030;
    }

    .crisis-close-btn:active {
      transform: scale(0.97);
    }

    .crisis-close-btn:focus-visible {
      outline: 3px solid #d4a843;
      outline-offset: 3px;
    }

    /* ── Responsive ── */
    @media (max-width: 480px) {
      .crisis-panel {
        padding: 1.5rem 1.25rem;
      }
      .crisis-title {
        font-size: 1.25rem;
      }
    }
  `;

  document.head.appendChild(style);
})();
