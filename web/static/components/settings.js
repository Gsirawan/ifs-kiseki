/**
 * IFS-Kiseki — Settings Component
 *
 * Responsibilities:
 *   - Render the settings form into #settings-container
 *   - Handle save (PUT /api/settings)
 *   - Apply theme/font-size changes immediately
 *   - Redacted API key handling (preserve existing keys)
 *
 * Exposed as a global `Settings` object (vanilla JS, no module system).
 * app.js calls Settings.render() when the user navigates to #settings.
 *
 * API used:
 *   GET  /api/settings    → config.Config (with API keys redacted)
 *   PUT  /api/settings    → config.Config (returns updated config)
 *   GET  /api/providers   → [{ name, model, has_key }]
 *   GET  /api/health      → { status, version }
 */

'use strict';

const Settings = (() => {

  // ── Private state ──────────────────────────────────────────────

  let _redactedKeys = { claude: '', grok: '' };
  let _rendered = false;

  // ── Public API ─────────────────────────────────────────────────

  /**
   * Render the settings page into #settings-container.
   * Fetches current config from the server.
   */
  async function render() {
    const container = document.getElementById('settings-container');
    if (!container) return;

    // Only render once — subsequent navigations reuse the DOM.
    if (_rendered) return;

    container.innerHTML = '<div class="settings-placeholder"><p>Loading settings…</p></div>';

    try {
      const [settings, providers, health] = await Promise.all([
        _fetch('/api/settings'),
        _fetch('/api/providers'),
        _fetchHealth(),
      ]);

      _redactedKeys.claude = settings.providers?.claude?.api_key || '';
      _redactedKeys.grok   = settings.providers?.grok?.api_key   || '';

      container.innerHTML = '';
      container.appendChild(_buildPage(settings, providers, health));
      _rendered = true;
    } catch (err) {
      container.innerHTML = '<div class="settings-placeholder"><p>Could not load settings.</p></div>';
      console.error('[settings] failed to load:', err);
    }
  }

  // ── Fetch helpers ──────────────────────────────────────────────

  async function _fetch(url) {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`${url} → HTTP ${res.status}`);
    return res.json();
  }

  async function _fetchHealth() {
    try {
      const res = await fetch('/api/health');
      if (!res.ok) return { version: 'unknown' };
      return res.json();
    } catch {
      return { version: 'unknown' };
    }
  }

  // ── Page builder ───────────────────────────────────────────────

  function _buildPage(settings, providers, health) {
    const page = document.createElement('div');
    page.className = 'settings-placeholder';
    page.style.overflowY = 'auto';
    page.style.maxHeight = '100%';

    const title = document.createElement('h2');
    title.textContent = 'Settings';
    page.appendChild(title);

    const form = document.createElement('form');
    form.id = 'settings-form';
    form.addEventListener('submit', e => {
      e.preventDefault();
      _handleSave(form, settings);
    });

    // Provider section
    form.appendChild(_buildProviderSection(settings, providers));
    // Companion section
    form.appendChild(_buildCompanionSection(settings));
    // Memory section
    form.appendChild(_buildMemorySection(settings));
    // Appearance section
    form.appendChild(_buildAppearanceSection(settings));
    // About section
    form.appendChild(_buildAboutSection(health));
    // Save button
    const saveBtn = document.createElement('button');
    saveBtn.type = 'submit';
    saveBtn.className = 'btn btn-primary';
    saveBtn.textContent = 'Save Settings';
    saveBtn.style.marginTop = '1.5rem';
    form.appendChild(saveBtn);

    const feedback = document.createElement('div');
    feedback.id = 'settings-feedback';
    feedback.style.marginTop = '0.5rem';
    feedback.style.fontSize = 'var(--font-size-sm)';
    feedback.setAttribute('aria-live', 'polite');
    form.appendChild(feedback);

    page.appendChild(form);
    return page;
  }

  // ── Section builders ───────────────────────────────────────────

  function _buildProviderSection(settings, providers) {
    const section = _section('Provider');

    section.appendChild(_field('Active Provider', _select('setting-provider', ['claude', 'grok'], settings.provider || 'claude')));

    // Claude key
    const claudeHasKey = providers.find(p => p.name === 'claude')?.has_key;
    section.appendChild(_field(
      'Claude API Key',
      _passwordInput('setting-claude-apikey', claudeHasKey ? _redactedKeys.claude : 'sk-ant-…'),
      claudeHasKey ? `Key configured (${_redactedKeys.claude}). Enter a new key to replace.` : 'No key set.'
    ));

    // Grok key
    const grokHasKey = providers.find(p => p.name === 'grok')?.has_key;
    section.appendChild(_field(
      'Grok API Key',
      _passwordInput('setting-grok-apikey', grokHasKey ? _redactedKeys.grok : 'xai-…'),
      grokHasKey ? `Key configured (${_redactedKeys.grok}). Enter a new key to replace.` : 'No key set.'
    ));

    return section;
  }

  function _buildCompanionSection(settings) {
    const c = settings.companion || {};
    const section = _section('Companion');

    section.appendChild(_field('Companion Name', _textInput('setting-companion-name', c.name || 'Kira')));
    section.appendChild(_field('Your Name', _textInput('setting-companion-username', c.user_name || ''), 'How your companion addresses you (optional).'));
    section.appendChild(_field('Focus Areas', _textInput('setting-companion-focus', (c.focus_areas || []).join(', ')), 'Comma-separated (e.g. anxiety, perfectionism).'));
    section.appendChild(_field('Custom Instructions', _textarea('setting-companion-instructions', c.custom_instructions || ''), 'Additional guidance for your companion.'));

    return section;
  }

  function _buildMemorySection(settings) {
    const m = settings.memory || {};
    const section = _section('Memory');

    section.appendChild(_field('Auto-save sessions', _checkbox('setting-memory-autosave', m.auto_save !== false)));
    section.appendChild(_field('Briefing on start', _checkbox('setting-memory-briefing', m.briefing_on_start !== false)));

    return section;
  }

  function _buildAppearanceSection(settings) {
    const ui = settings.ui || {};
    const section = _section('Appearance');

    section.appendChild(_field('Theme', _select('setting-ui-theme', ['warm', 'dark'], ui.theme || 'warm')));
    section.appendChild(_field('Font Size', _select('setting-ui-fontsize', ['small', 'medium', 'large'], ui.font_size || 'medium')));

    return section;
  }

  function _buildAboutSection(health) {
    const section = _section('About');
    const p = document.createElement('p');
    p.style.color = 'var(--text-secondary)';
    p.style.fontSize = 'var(--font-size-sm)';
    p.innerHTML = `IFS-Kiseki v${_esc(health.version || 'unknown')}<br>` +
      'An IFS self-exploration companion. Not a substitute for professional therapy.';
    section.appendChild(p);
    return section;
  }

  // ── Save handler ───────────────────────────────────────────────

  async function _handleSave(form, original) {
    const saveBtn = form.querySelector('button[type="submit"]');
    const feedback = document.getElementById('settings-feedback');

    if (saveBtn) { saveBtn.disabled = true; saveBtn.textContent = 'Saving…'; }
    if (feedback) { feedback.textContent = ''; feedback.style.color = ''; }

    try {
      const payload = _collectValues(original);

      const res = await fetch('/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const updated = await res.json();

      _redactedKeys.claude = updated.providers?.claude?.api_key || '';
      _redactedKeys.grok   = updated.providers?.grok?.api_key   || '';

      // Clear key inputs
      ['claude', 'grok'].forEach(name => {
        const input = document.getElementById(`setting-${name}-apikey`);
        if (input) { input.value = ''; input.placeholder = _redactedKeys[name] || 'sk-…'; }
      });

      if (feedback) { feedback.textContent = 'Settings saved.'; feedback.style.color = 'var(--status-ok)'; }

      // Apply theme/font immediately
      if (updated.ui) {
        if (updated.ui.theme) document.documentElement.setAttribute('data-theme', updated.ui.theme);
        if (updated.ui.font_size) document.documentElement.setAttribute('data-font-size', updated.ui.font_size);
      }
    } catch (err) {
      if (feedback) { feedback.textContent = 'Could not save: ' + err.message; feedback.style.color = 'var(--status-error)'; }
    } finally {
      if (saveBtn) { saveBtn.disabled = false; saveBtn.textContent = 'Save Settings'; }
    }
  }

  function _collectValues(original) {
    const get = id => { const el = document.getElementById(id); return el ? el.value : ''; };
    const getChecked = id => { const el = document.getElementById(id); return el ? el.checked : false; };

    const resolveKey = (inputId, name) => {
      const val = get(inputId).trim();
      if (!val || val === _redactedKeys[name]) return '';
      return val;
    };

    const focusAreas = get('setting-companion-focus')
      .split(',').map(s => s.trim()).filter(s => s.length > 0);

    return {
      version: original.version || 1,
      provider: get('setting-provider') || original.provider || 'claude',
      providers: {
        claude: { ...(original.providers?.claude || {}), api_key: resolveKey('setting-claude-apikey', 'claude') },
        grok:   { ...(original.providers?.grok || {}),   api_key: resolveKey('setting-grok-apikey', 'grok') },
      },
      embeddings: original.embeddings || {},
      server: original.server || {},
      companion: {
        name: get('setting-companion-name').trim() || 'Kira',
        user_name: get('setting-companion-username').trim(),
        focus_areas: focusAreas,
        custom_instructions: get('setting-companion-instructions').trim(),
      },
      crisis: original.crisis || {},
      memory: {
        auto_save: getChecked('setting-memory-autosave'),
        briefing_on_start: getChecked('setting-memory-briefing'),
        max_context_chunks: original.memory?.max_context_chunks ?? 5,
      },
      ui: {
        theme: get('setting-ui-theme') || 'warm',
        font_size: get('setting-ui-fontsize') || 'medium',
      },
    };
  }

  // ── Form element helpers ───────────────────────────────────────

  function _section(title) {
    const s = document.createElement('fieldset');
    s.style.border = 'none';
    s.style.padding = '0';
    s.style.marginBottom = '1.5rem';
    const legend = document.createElement('legend');
    legend.style.fontWeight = '600';
    legend.style.fontSize = 'var(--font-size-lg)';
    legend.style.marginBottom = '0.75rem';
    legend.style.color = 'var(--text-primary)';
    legend.textContent = title;
    s.appendChild(legend);
    return s;
  }

  function _field(label, input, hint) {
    const row = document.createElement('div');
    row.style.marginBottom = '1rem';

    const lbl = document.createElement('label');
    lbl.style.display = 'block';
    lbl.style.fontWeight = '500';
    lbl.style.fontSize = 'var(--font-size-sm)';
    lbl.style.marginBottom = '0.25rem';
    lbl.style.color = 'var(--text-primary)';
    lbl.textContent = label;
    if (input.id) lbl.htmlFor = input.id;
    row.appendChild(lbl);

    if (hint) {
      const h = document.createElement('p');
      h.style.fontSize = 'var(--font-size-xs)';
      h.style.color = 'var(--text-muted)';
      h.style.marginBottom = '0.25rem';
      h.textContent = hint;
      row.appendChild(h);
    }

    row.appendChild(input);
    return row;
  }

  function _textInput(id, value) {
    const input = document.createElement('input');
    input.type = 'text'; input.id = id; input.value = value;
    _styleInput(input);
    return input;
  }

  function _passwordInput(id, placeholder) {
    const input = document.createElement('input');
    input.type = 'password'; input.id = id; input.placeholder = placeholder;
    input.autocomplete = 'off'; input.spellcheck = false;
    _styleInput(input);
    return input;
  }

  function _textarea(id, value) {
    const ta = document.createElement('textarea');
    ta.id = id; ta.rows = 3; ta.value = value;
    _styleInput(ta);
    return ta;
  }

  function _select(id, options, selected) {
    const sel = document.createElement('select');
    sel.id = id;
    _styleInput(sel);
    options.forEach(opt => {
      const el = document.createElement('option');
      el.value = opt;
      el.textContent = opt.charAt(0).toUpperCase() + opt.slice(1);
      if (opt === selected) el.selected = true;
      sel.appendChild(el);
    });
    return sel;
  }

  function _checkbox(id, checked) {
    const input = document.createElement('input');
    input.type = 'checkbox'; input.id = id; input.checked = checked;
    return input;
  }

  function _styleInput(el) {
    el.style.width = '100%';
    el.style.padding = 'var(--space-sm) var(--space-md)';
    el.style.background = 'var(--bg-input)';
    el.style.border = '1px solid var(--border)';
    el.style.borderRadius = 'var(--radius-md)';
    el.style.fontFamily = 'var(--font-body)';
    el.style.fontSize = '1rem';
    el.style.color = 'var(--text-primary)';
  }

  function _esc(str) {
    if (!str) return '';
    return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // ── Expose ─────────────────────────────────────────────────────

  return { render };

})();
