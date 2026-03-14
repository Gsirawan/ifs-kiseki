/**
 * IFS-Kiseki — Chat Component
 *
 * Responsibilities:
 *   - Render the chat view (message list + input area)
 *   - Add user and assistant message bubbles
 *   - Handle streaming: thinking indicator → streaming bubble → finalised bubble
 *   - Markdown rendering (bold, italic, lists, code, blockquote) — no library
 *   - Auto-scroll to bottom on new content
 *   - Input handling: Enter to send, Shift+Enter for newline, disable while streaming
 *
 * Exposed as a global `Chat` object (no module system — vanilla JS).
 * app.js calls Chat.init() and Chat.* methods.
 */

'use strict';

const Chat = (() => {

  // ── Private state ──────────────────────────────────────────────

  let _onSend = null;           // callback(content: string) — provided by app.js
  let _streamingBubble = null;  // the <div> currently receiving streamed tokens
  let _streamingContent = '';   // raw accumulated text during streaming
  let _thinkingEl = null;       // the thinking indicator element
  let _companionName = 'Kira';  // companion display name — updated from server settings

  // ── DOM references (set in init) ──────────────────────────────

  let _messageList = null;
  let _input = null;
  let _sendBtn = null;
  let _form = null;

  // ── Markdown renderer ──────────────────────────────────────────

  /**
   * Convert a plain-text string with basic markdown into safe HTML.
   *
   * Supported:
   *   - **bold** and __bold__
   *   - *italic* and _italic_
   *   - `inline code`
   *   - ```code blocks```
   *   - > blockquote lines
   *   - - / * / + unordered list items
   *   - 1. ordered list items
   *   - Blank lines → paragraph breaks
   *
   * Security: all raw text is HTML-escaped before pattern substitution.
   * We never inject user-supplied HTML.
   */
  function renderMarkdown(text) {
    if (!text) return '';

    // 1. Escape HTML entities in the raw text first.
    let html = escapeHTML(text);

    // 2. Code blocks (``` ... ```) — must run before inline code.
    html = html.replace(/```[\w]*\n?([\s\S]*?)```/g, (_, code) => {
      return `<pre><code>${code.trim()}</code></pre>`;
    });

    // 3. Inline code — `code`.
    html = html.replace(/`([^`\n]+)`/g, '<code>$1</code>');

    // 4. Blockquotes — lines starting with &gt; (escaped >).
    html = html.replace(/^&gt;\s?(.+)$/gm, '<blockquote>$1</blockquote>');

    // 5. Bold — **text** or __text__.
    html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/__(.+?)__/g, '<strong>$1</strong>');

    // 6. Italic — *text* or _text_ (not inside words).
    html = html.replace(/(?<!\*)\*(?!\*)(.+?)(?<!\*)\*(?!\*)/g, '<em>$1</em>');
    html = html.replace(/(?<!_)_(?!_)(.+?)(?<!_)_(?!_)/g, '<em>$1</em>');

    // 7. Unordered lists — lines starting with - / * / +.
    //    Group consecutive list lines into a <ul>.
    html = processListBlock(html, /^[-*+]\s+(.+)$/gm, 'ul');

    // 8. Ordered lists — lines starting with N.
    html = processListBlock(html, /^\d+\.\s+(.+)$/gm, 'ol');

    // 9. Paragraphs — split on blank lines, wrap non-block content in <p>.
    html = paragraphise(html);

    return html;
  }

  /**
   * Escape HTML special characters.
   */
  function escapeHTML(str) {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  /**
   * Find consecutive lines matching `lineRegex` and wrap them in a list element.
   * This is a simplified approach: we replace each matching line with an <li>,
   * then wrap consecutive <li> runs in the given list tag.
   */
  function processListBlock(html, lineRegex, tag) {
    // Replace each matching line with a bare <li>.
    html = html.replace(lineRegex, (_, content) => `<li>${content}</li>`);

    // Wrap consecutive <li> sequences in the list tag.
    html = html.replace(/(<li>[\s\S]*?<\/li>)(\n<li>[\s\S]*?<\/li>)*/g, (match) => {
      return `<${tag}>${match}</${tag}>`;
    });

    return html;
  }

  /**
   * Split on blank lines and wrap non-block-level content in <p> tags.
   * Block-level elements (pre, ul, ol, blockquote) are left unwrapped.
   */
  function paragraphise(html) {
    const blockTags = /^<(pre|ul|ol|blockquote|li|h[1-6])/;
    const parts = html.split(/\n{2,}/);

    return parts.map((part) => {
      const trimmed = part.trim();
      if (!trimmed) return '';
      if (blockTags.test(trimmed)) return trimmed;
      // Replace single newlines within a paragraph with <br>.
      return `<p>${trimmed.replace(/\n/g, '<br>')}</p>`;
    }).filter(Boolean).join('\n');
  }

  // ── DOM helpers ────────────────────────────────────────────────

  /**
   * Scroll the message list to the bottom.
   */
  function scrollToBottom() {
    if (_messageList) {
      _messageList.scrollTop = _messageList.scrollHeight;
    }
  }

  /**
   * Create the message list and input area DOM and inject into #chat-container.
   */
  function buildChatDOM() {
    const container = document.getElementById('chat-container');
    if (!container) {
      console.error('[chat] #chat-container not found');
      return;
    }

    // Message list.
    const messageList = document.createElement('div');
    messageList.id = 'message-list';
    messageList.setAttribute('role', 'log');
    messageList.setAttribute('aria-live', 'polite');
    messageList.setAttribute('aria-label', 'Conversation');

    // Input area.
    const inputArea = document.createElement('div');
    inputArea.id = 'input-area';
    inputArea.innerHTML = `
      <form id="input-form" autocomplete="off">
        <textarea
          id="message-input"
          rows="1"
          placeholder="Share what's on your mind…"
          aria-label="Message input"
          aria-multiline="true"
        ></textarea>
        <button type="submit" id="send-btn" aria-label="Send message">Send</button>
      </form>
      <p id="input-hint">Enter to send &nbsp;·&nbsp; Shift+Enter for new line</p>
    `;

    container.appendChild(messageList);
    container.appendChild(inputArea);

    // Cache references.
    _messageList = messageList;
    _input = document.getElementById('message-input');
    _sendBtn = document.getElementById('send-btn');
    _form = document.getElementById('input-form');
  }

  /**
   * Wire form submission and keyboard shortcuts.
   */
  function bindInputEvents() {
    if (!_form || !_input) return;

    _form.addEventListener('submit', (e) => {
      e.preventDefault();
      submitInput();
    });

    _input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        submitInput();
      }
    });

    // Auto-resize textarea as user types.
    _input.addEventListener('input', autoResizeTextarea);
  }

  /**
   * Read the input value, validate, and fire the onSend callback.
   */
  function submitInput() {
    if (!_input || !_onSend) return;
    const content = _input.value.trim();
    if (!content) return;

    _input.value = '';
    autoResizeTextarea();
    _onSend(content);
  }

  /**
   * Grow the textarea to fit its content (up to CSS max-height).
   */
  function autoResizeTextarea() {
    if (!_input) return;
    _input.style.height = 'auto';
    _input.style.height = `${_input.scrollHeight}px`;
  }

  // ── Message rendering ──────────────────────────────────────────

  /**
   * Create a message wrapper element.
   * @param {'user'|'assistant'} role
   * @returns {HTMLElement} the outer .message div
   */
  function createMessageEl(role) {
    const wrapper = document.createElement('div');
    wrapper.className = `message message-${role}`;
    wrapper.setAttribute('role', 'listitem');

    const roleLabel = document.createElement('span');
    roleLabel.className = 'message-role';
    roleLabel.textContent = role === 'user' ? 'You' : _companionName;
    roleLabel.setAttribute('aria-hidden', 'true');

    const bubble = document.createElement('div');
    bubble.className = 'message-bubble';

    wrapper.appendChild(roleLabel);
    wrapper.appendChild(bubble);
    return wrapper;
  }

  // ── Public API ─────────────────────────────────────────────────

  /**
   * Initialise the chat component.
   * Must be called once before any other Chat.* method.
   * @param {{ onSend: function }} options
   */
  function init(options) {
    _onSend = options.onSend || null;
    buildChatDOM();
    bindInputEvents();
    // Fetch companion name from server settings (non-blocking).
    _fetchCompanionName();
  }

  /**
   * Fetch the companion name from server settings.
   * Falls back to "Kira" if the fetch fails.
   */
  async function _fetchCompanionName() {
    try {
      const res = await fetch('/api/settings');
      if (!res.ok) return;
      const cfg = await res.json();
      if (cfg.companion && cfg.companion.name) {
        _companionName = cfg.companion.name;
      }
    } catch (_) {
      // Non-critical — default name is fine.
    }
  }

  /**
   * Add a finalised message bubble to the conversation.
   * @param {'user'|'assistant'} role
   * @param {string} content  Plain text (user) or markdown (assistant)
   */
  function addMessage(role, content) {
    if (!_messageList) return;

    const wrapper = createMessageEl(role);
    const bubble = wrapper.querySelector('.message-bubble');

    if (role === 'assistant') {
      bubble.innerHTML = renderMarkdown(content);
    } else {
      // User messages: plain text, preserve newlines.
      bubble.textContent = content;
    }

    _messageList.appendChild(wrapper);
    scrollToBottom();
  }

  /**
   * Show the "Thinking…" indicator while waiting for the first token.
   */
  function showThinking() {
    if (!_messageList) return;
    removeThinking(); // guard against duplicates

    const el = document.createElement('div');
    el.id = 'thinking-indicator';
    el.className = 'message message-assistant thinking-indicator';
    el.setAttribute('aria-live', 'polite');
    el.setAttribute('aria-label', `${_companionName} is thinking`);
    el.innerHTML = `
      <span class="thinking-dots" aria-hidden="true">
        <span></span><span></span><span></span>
      </span>
      <span>Thinking…</span>
    `;

    _messageList.appendChild(el);
    _thinkingEl = el;
    scrollToBottom();
  }

  /**
   * Remove the thinking indicator if present.
   */
  function removeThinking() {
    if (_thinkingEl) {
      _thinkingEl.remove();
      _thinkingEl = null;
    }
    // Also remove by ID in case of stale reference.
    const stale = document.getElementById('thinking-indicator');
    if (stale) stale.remove();
  }

  /**
   * Replace the thinking indicator with an empty streaming bubble.
   * Subsequent appendDelta() calls fill it in.
   */
  function startStreaming() {
    removeThinking();

    if (!_messageList) return;

    const wrapper = createMessageEl('assistant');
    wrapper.id = 'streaming-message';
    const bubble = wrapper.querySelector('.message-bubble');
    bubble.id = 'streaming-bubble';

    _streamingBubble = bubble;
    _streamingContent = '';

    _messageList.appendChild(wrapper);
    scrollToBottom();
  }

  /**
   * Append a token delta to the active streaming bubble.
   * Re-renders markdown on each delta so formatting appears progressively.
   * @param {string} text
   */
  function appendDelta(text) {
    if (!_streamingBubble) return;

    _streamingContent += text;

    // Render markdown progressively. This is slightly expensive but keeps
    // the output readable as it streams — worth it for therapy context.
    _streamingBubble.innerHTML = renderMarkdown(_streamingContent);
    scrollToBottom();
  }

  /**
   * Finalise the streaming bubble. Adds token usage metadata if provided.
   * @param {{ input: number, output: number }|null} usage
   */
  function endStreaming(usage) {
    if (_streamingBubble) {
      // Final render pass — ensures clean markdown.
      _streamingBubble.innerHTML = renderMarkdown(_streamingContent);

      if (usage) {
        const meta = document.createElement('div');
        meta.className = 'message-meta';
        meta.setAttribute('aria-label', `Token usage: ${usage.input} input, ${usage.output} output`);
        meta.textContent = `${usage.input + usage.output} tokens`;
        _streamingBubble.parentElement.appendChild(meta);
      }

      _streamingBubble = null;
      _streamingContent = '';
    }

    // Remove streaming ID from wrapper.
    const wrapper = document.getElementById('streaming-message');
    if (wrapper) wrapper.removeAttribute('id');

    scrollToBottom();
  }

  /**
   * Show an error message in the conversation flow.
   * @param {string} message
   */
  function showError(message) {
    if (!_messageList) return;

    const banner = document.createElement('div');
    banner.className = 'error-banner';
    banner.setAttribute('role', 'alert');
    banner.textContent = message;

    _messageList.appendChild(banner);
    scrollToBottom();
  }

  /**
   * Enable or disable the input textarea and send button.
   * @param {boolean} enabled
   */
  function setInputEnabled(enabled) {
    if (_input) _input.disabled = !enabled;
    if (_sendBtn) _sendBtn.disabled = !enabled;
    if (enabled && _input) _input.focus();
  }

  /**
   * Clear all messages from the conversation (new session).
   */
  function clearMessages() {
    if (_messageList) {
      _messageList.innerHTML = '';
    }
    _streamingBubble = null;
    _streamingContent = '';
    _thinkingEl = null;
  }

  // ── Expose public API ──────────────────────────────────────────

  return {
    init,
    addMessage,
    showThinking,
    removeThinking,
    startStreaming,
    appendDelta,
    endStreaming,
    showError,
    setInputEnabled,
    clearMessages,
  };

})();
