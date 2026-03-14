/**
 * onboarding.js — First-launch onboarding overlay for IFS-Kiseki.
 *
 * Two-step flow:
 *   Step 1: Disclaimer — user must check all three boxes before continuing.
 *   Step 2: API Key Entry — provider selection, key input, connection test.
 *
 * Exports:
 *   Onboarding.init()   — check disclaimer status and show overlay if needed.
 *   Onboarding.show()   — force-show the overlay (for testing).
 *
 * API used:
 *   GET  /api/settings          → { disclaimer_accepted: bool, ... }
 *   POST /api/accept-disclaimer → { accepted: true }
 *   POST /api/test-provider     → { success: bool, error?: string }
 *   PUT  /api/settings          → updated config (to save API key)
 *
 * Design intent:
 *   Warm, professional, caring. This is the first thing a user sees.
 *   They are here because they need help. Make them feel safe, not scared.
 *   The disclaimer is honest and clear — not a wall of legal text.
 */

'use strict';

// ── Module state ─────────────────────────────────────────────────

const _state = {
  step: 1,                  // current step (1 = disclaimer, 2 = api key)
  overlay: null,            // the overlay DOM element
  onComplete: null,         // callback when onboarding finishes
  selectedProvider: 'claude', // 'claude' or 'grok'
  testPassed: false,        // true after successful connection test
  skipped: false,           // true if user chose "Skip for now"
};

// ── Public API ────────────────────────────────────────────────────

const Onboarding = {
  /**
   * Check disclaimer status and show the overlay if not yet accepted.
   * @param {Function} onComplete — called when onboarding is done
   */
  async init(onComplete) {
    _state.onComplete = onComplete || (() => {});

    try {
      const res = await fetch('/api/settings');
      if (!res.ok) {
        // Server error — proceed without onboarding to avoid blocking the user.
        console.warn('[onboarding] could not fetch settings, skipping onboarding');
        _state.onComplete();
        return;
      }
      const cfg = await res.json();

      if (cfg.disclaimer_accepted) {
        // Already accepted — proceed directly to the app.
        _state.onComplete();
        return;
      }
    } catch (err) {
      // Network error — proceed without blocking.
      console.warn('[onboarding] network error checking disclaimer:', err);
      _state.onComplete();
      return;
    }

    // Disclaimer not yet accepted — show the onboarding overlay.
    _show();
  },

  /** Force-show the overlay (useful for testing or re-running onboarding). */
  show() {
    _show();
  },
};

// ── Overlay lifecycle ─────────────────────────────────────────────

function _show() {
  if (_state.overlay) return; // already visible

  _state.step = 1;
  _state.testPassed = false;
  _state.skipped = false;
  _state.selectedProvider = 'claude';

  const overlay = _buildOverlay();
  _state.overlay = overlay;
  document.body.appendChild(overlay);

  // Prevent body scroll while overlay is open.
  document.body.style.overflow = 'hidden';

  // Animate in.
  requestAnimationFrame(() => {
    overlay.classList.add('onboarding-overlay--visible');
  });
}

function _dismiss() {
  const overlay = _state.overlay;
  if (!overlay) return;

  overlay.classList.remove('onboarding-overlay--visible');

  // Wait for fade-out transition, then remove from DOM.
  overlay.addEventListener('transitionend', () => {
    overlay.remove();
    _state.overlay = null;
    document.body.style.overflow = '';
    if (_state.onComplete) {
      _state.onComplete();
    }
  }, { once: true });
}

// ── Overlay shell ─────────────────────────────────────────────────

function _buildOverlay() {
  const overlay = document.createElement('div');
  overlay.className = 'onboarding-overlay';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-labelledby', 'onboarding-title');

  const card = document.createElement('div');
  card.className = 'onboarding-card';
  card.id = 'onboarding-card';

  overlay.appendChild(card);
  _renderStep(card, 1);

  return overlay;
}

// ── Step rendering ────────────────────────────────────────────────

function _renderStep(card, step) {
  // Fade out current content, swap, fade in.
  card.style.opacity = '0';
  card.style.transform = 'translateY(8px)';

  setTimeout(() => {
    card.innerHTML = '';

    if (step === 1) {
      card.appendChild(_buildStepIndicator(1, 2));
      card.appendChild(_buildDisclaimerStep());
    } else {
      card.appendChild(_buildStepIndicator(2, 2));
      card.appendChild(_buildAPIKeyStep());
    }

    // Animate in.
    requestAnimationFrame(() => {
      card.style.opacity = '1';
      card.style.transform = 'translateY(0)';
    });
  }, 180);
}

// ── Step indicator ────────────────────────────────────────────────

function _buildStepIndicator(current, total) {
  const indicator = document.createElement('div');
  indicator.className = 'onboarding-step-indicator';
  indicator.setAttribute('aria-label', `Step ${current} of ${total}`);

  for (let i = 1; i <= total; i++) {
    const dot = document.createElement('span');
    dot.className = 'onboarding-step-dot' + (i === current ? ' onboarding-step-dot--active' : '');
    dot.setAttribute('aria-hidden', 'true');
    indicator.appendChild(dot);
  }

  const label = document.createElement('span');
  label.className = 'onboarding-step-label';
  label.textContent = `Step ${current} of ${total}`;
  indicator.appendChild(label);

  return indicator;
}

// ── Step 1: Disclaimer ────────────────────────────────────────────

function _buildDisclaimerStep() {
  const container = document.createElement('div');
  container.className = 'onboarding-step';

  // Header.
  const header = document.createElement('div');
  header.className = 'onboarding-header';
  header.innerHTML = `
    <div class="onboarding-logo" aria-hidden="true">🌿</div>
    <h1 class="onboarding-title" id="onboarding-title">Welcome to IFS-Kiseki</h1>
    <p class="onboarding-subtitle">
      A private space for IFS self-exploration. Before you begin, please read the following.
    </p>
  `;
  container.appendChild(header);

  // Disclaimer content.
  const disclaimer = document.createElement('div');
  disclaimer.className = 'onboarding-disclaimer';
  disclaimer.setAttribute('role', 'region');
  disclaimer.setAttribute('aria-label', 'Disclaimer');
  disclaimer.innerHTML = `
    <div class="disclaimer-item">
      <div class="disclaimer-item__icon" aria-hidden="true">⚕️</div>
      <div class="disclaimer-item__content">
        <strong>This is not therapy</strong>
        <p>IFS-Kiseki is an AI-powered self-exploration tool informed by Internal Family Systems (IFS) principles. It is <em>not</em> a substitute for professional mental health care, therapy, or medical advice.</p>
      </div>
    </div>

    <div class="disclaimer-item">
      <div class="disclaimer-item__icon" aria-hidden="true">🤖</div>
      <div class="disclaimer-item__content">
        <strong>You are talking to AI</strong>
        <p>Your companion is an artificial intelligence — not a licensed therapist, counselor, or medical professional. Its responses are generated by a language model.</p>
      </div>
    </div>

    <div class="disclaimer-item">
      <div class="disclaimer-item__icon" aria-hidden="true">🪞</div>
      <div class="disclaimer-item__content">
        <strong>For self-exploration only</strong>
        <p>This tool supports personal reflection and journaling-style exploration of your inner world. It does not provide diagnosis, treatment, or clinical intervention.</p>
      </div>
    </div>

    <div class="disclaimer-item disclaimer-item--crisis">
      <div class="disclaimer-item__icon" aria-hidden="true">🆘</div>
      <div class="disclaimer-item__content">
        <strong>Crisis situations</strong>
        <p>If you are experiencing a mental health emergency, thoughts of self-harm, or thoughts of harming others, please contact emergency services immediately:</p>
        <ul class="crisis-resources">
          <li><strong>988 Suicide &amp; Crisis Lifeline (US):</strong> Call or text <strong>988</strong></li>
          <li><strong>Emergency Services:</strong> Call <strong>911</strong></li>
          <li><strong>Crisis Text Line:</strong> Text HOME to <strong>741741</strong></li>
        </ul>
      </div>
    </div>

    <div class="disclaimer-item">
      <div class="disclaimer-item__icon" aria-hidden="true">🌊</div>
      <div class="disclaimer-item__content">
        <strong>Deep trauma work</strong>
        <p>IFS work with exiles (deeply wounded parts) can surface intense emotions. For deep trauma work, we strongly recommend working with a trained IFS therapist. This tool is best suited for protector work and self-awareness.</p>
      </div>
    </div>

    <div class="disclaimer-item">
      <div class="disclaimer-item__icon" aria-hidden="true">🔒</div>
      <div class="disclaimer-item__content">
        <strong>Your data is local</strong>
        <p>All your conversations and data are stored locally on your computer. The only external communication is with your chosen AI provider (Anthropic/xAI) to generate responses.</p>
      </div>
    </div>

    <div class="disclaimer-item">
      <div class="disclaimer-item__icon" aria-hidden="true">🔞</div>
      <div class="disclaimer-item__content">
        <strong>Age requirement</strong>
        <p>You must be at least 18 years old to use this tool.</p>
      </div>
    </div>
  `;
  container.appendChild(disclaimer);

  // Checkboxes.
  const checkboxSection = document.createElement('div');
  checkboxSection.className = 'onboarding-checkboxes';
  checkboxSection.setAttribute('role', 'group');
  checkboxSection.setAttribute('aria-label', 'Acknowledgements — all required');

  const checks = [
    { id: 'check-read',    label: 'I have read and understand this disclaimer' },
    { id: 'check-age',     label: 'I am at least 18 years old' },
    { id: 'check-therapy', label: 'I understand this is not a substitute for professional therapy' },
  ];

  checks.forEach(({ id, label }) => {
    const row = document.createElement('label');
    row.className = 'onboarding-checkbox-row';
    row.htmlFor = id;

    const input = document.createElement('input');
    input.type      = 'checkbox';
    input.id        = id;
    input.className = 'onboarding-checkbox';
    input.addEventListener('change', _updateContinueButton);

    const mark = document.createElement('span');
    mark.className = 'onboarding-checkbox-mark';
    mark.setAttribute('aria-hidden', 'true');

    const text = document.createElement('span');
    text.className   = 'onboarding-checkbox-label';
    text.textContent = label;

    row.appendChild(input);
    row.appendChild(mark);
    row.appendChild(text);
    checkboxSection.appendChild(row);
  });

  container.appendChild(checkboxSection);

  // Continue button.
  const actions = document.createElement('div');
  actions.className = 'onboarding-actions';

  const continueBtn = document.createElement('button');
  continueBtn.id        = 'onboarding-continue';
  continueBtn.className = 'onboarding-btn onboarding-btn--primary';
  continueBtn.textContent = 'Continue →';
  continueBtn.disabled  = true;
  continueBtn.setAttribute('aria-describedby', 'onboarding-continue-hint');
  continueBtn.addEventListener('click', _handleDisclaimerAccept);

  const hint = document.createElement('p');
  hint.id          = 'onboarding-continue-hint';
  hint.className   = 'onboarding-btn-hint';
  hint.textContent = 'Please check all boxes to continue.';

  actions.appendChild(continueBtn);
  actions.appendChild(hint);
  container.appendChild(actions);

  return container;
}

function _updateContinueButton() {
  const checks = ['check-read', 'check-age', 'check-therapy'];
  const allChecked = checks.every(id => {
    const el = document.getElementById(id);
    return el && el.checked;
  });

  const btn  = document.getElementById('onboarding-continue');
  const hint = document.getElementById('onboarding-continue-hint');

  if (btn) {
    btn.disabled = !allChecked;
  }
  if (hint) {
    hint.textContent = allChecked
      ? 'All acknowledged. You may continue.'
      : 'Please check all boxes to continue.';
    hint.className = allChecked
      ? 'onboarding-btn-hint onboarding-btn-hint--ready'
      : 'onboarding-btn-hint';
  }
}

async function _handleDisclaimerAccept() {
  const btn = document.getElementById('onboarding-continue');
  if (btn) {
    btn.disabled    = true;
    btn.textContent = 'Saving…';
  }

  try {
    const res = await fetch('/api/accept-disclaimer', { method: 'POST' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);

    // Move to step 2.
    _state.step = 2;
    const card = document.getElementById('onboarding-card');
    if (card) _renderStep(card, 2);

  } catch (err) {
    console.error('[onboarding] failed to accept disclaimer:', err);
    if (btn) {
      btn.disabled    = false;
      btn.textContent = 'Continue →';
    }
    _showStepError('Could not save your acceptance. Please try again.');
  }
}

// ── Step 2: API Key Entry ─────────────────────────────────────────

function _buildAPIKeyStep() {
  const container = document.createElement('div');
  container.className = 'onboarding-step';

  // Header.
  const header = document.createElement('div');
  header.className = 'onboarding-header';
  header.innerHTML = `
    <div class="onboarding-logo" aria-hidden="true">🔑</div>
    <h1 class="onboarding-title" id="onboarding-title">Connect Your AI Provider</h1>
    <p class="onboarding-subtitle">
      IFS-Kiseki uses your own API key — your conversations stay private and go directly to the provider.
    </p>
  `;
  container.appendChild(header);

  // Provider selection.
  const providerSection = document.createElement('div');
  providerSection.className = 'onboarding-provider-select';

  const providerLabel = document.createElement('p');
  providerLabel.className   = 'onboarding-field-label';
  providerLabel.textContent = 'Choose your provider:';
  providerSection.appendChild(providerLabel);

  const providerCards = document.createElement('div');
  providerCards.className = 'onboarding-provider-cards';
  providerCards.setAttribute('role', 'radiogroup');
  providerCards.setAttribute('aria-label', 'AI provider selection');

  const providers = [
    {
      id:          'provider-claude',
      value:       'claude',
      name:        'Claude',
      badge:       'Recommended',
      description: 'Anthropic\'s Claude — thoughtful, nuanced, excellent for IFS work.',
      keyHint:     'Get your API key at console.anthropic.com',
      keyLink:     'https://console.anthropic.com',
      placeholder: 'sk-ant-…',
    },
    {
      id:          'provider-grok',
      value:       'grok',
      name:        'Grok',
      badge:       'Premium',
      description: 'xAI\'s Grok — direct, perceptive, powerful for deep inner work.',
      keyHint:     'Get your API key at console.x.ai',
      keyLink:     'https://console.x.ai',
      placeholder: 'xai-…',
    },
  ];

  providers.forEach(p => {
    const card = document.createElement('label');
    card.className = 'onboarding-provider-card' + (p.value === _state.selectedProvider ? ' onboarding-provider-card--selected' : '');
    card.htmlFor   = p.id;

    const radio = document.createElement('input');
    radio.type    = 'radio';
    radio.id      = p.id;
    radio.name    = 'provider';
    radio.value   = p.value;
    radio.checked = p.value === _state.selectedProvider;
    radio.className = 'onboarding-provider-radio';
    radio.addEventListener('change', () => _handleProviderChange(p.value));

    const cardContent = document.createElement('div');
    cardContent.className = 'onboarding-provider-card__content';
    cardContent.innerHTML = `
      <div class="onboarding-provider-card__header">
        <span class="onboarding-provider-card__name">${_escapeHtml(p.name)}</span>
        <span class="onboarding-provider-card__badge">${_escapeHtml(p.badge)}</span>
      </div>
      <p class="onboarding-provider-card__desc">${_escapeHtml(p.description)}</p>
    `;

    card.appendChild(radio);
    card.appendChild(cardContent);
    providerCards.appendChild(card);
  });

  providerSection.appendChild(providerCards);
  container.appendChild(providerSection);

  // API key input.
  const keySection = document.createElement('div');
  keySection.className = 'onboarding-key-section';
  keySection.id        = 'onboarding-key-section';

  _buildKeySection(keySection, _state.selectedProvider, providers);
  container.appendChild(keySection);

  // Test result area.
  const testResult = document.createElement('div');
  testResult.id        = 'onboarding-test-result';
  testResult.className = 'onboarding-test-result';
  testResult.setAttribute('aria-live', 'polite');
  testResult.setAttribute('role', 'status');
  container.appendChild(testResult);

  // Actions.
  const actions = document.createElement('div');
  actions.className = 'onboarding-actions onboarding-actions--api';

  const startBtn = document.createElement('button');
  startBtn.id        = 'onboarding-start';
  startBtn.className = 'onboarding-btn onboarding-btn--primary';
  startBtn.textContent = 'Start Exploring';
  startBtn.disabled  = true;
  startBtn.addEventListener('click', _handleStart);

  const skipBtn = document.createElement('button');
  skipBtn.id        = 'onboarding-skip';
  skipBtn.className = 'onboarding-btn onboarding-btn--ghost';
  skipBtn.textContent = 'Skip for now';
  skipBtn.setAttribute('aria-label', 'Skip API key setup — you can add it later in Settings');
  skipBtn.addEventListener('click', _handleSkip);

  actions.appendChild(startBtn);
  actions.appendChild(skipBtn);
  container.appendChild(actions);

  const skipHint = document.createElement('p');
  skipHint.className   = 'onboarding-skip-hint';
  skipHint.textContent = 'You can add your API key later in Settings.';
  container.appendChild(skipHint);

  return container;
}

function _buildKeySection(container, providerValue, providers) {
  const p = providers.find(x => x.value === providerValue) || providers[0];

  container.innerHTML = `
    <label class="onboarding-field-label" for="onboarding-api-key">
      API Key
    </label>
    <p class="onboarding-field-hint">
      <a href="${_escapeHtml(p.keyLink)}" target="_blank" rel="noopener noreferrer"
         class="onboarding-link">${_escapeHtml(p.keyHint)}</a>
    </p>
    <div class="onboarding-key-input-row">
      <input
        type="password"
        id="onboarding-api-key"
        class="onboarding-input"
        placeholder="${_escapeHtml(p.placeholder)}"
        autocomplete="off"
        spellcheck="false"
        aria-label="API key for ${_escapeHtml(p.name)}"
      />
      <button
        id="onboarding-test-btn"
        class="onboarding-btn onboarding-btn--secondary"
        type="button"
        aria-label="Test connection with this API key"
      >
        Test Connection
      </button>
    </div>
  `;

  // Wire the test button.
  const testBtn = container.querySelector('#onboarding-test-btn');
  if (testBtn) {
    testBtn.addEventListener('click', _handleTestConnection);
  }

  // Wire key input — enable start button if test already passed and key unchanged.
  const keyInput = container.querySelector('#onboarding-api-key');
  if (keyInput) {
    keyInput.addEventListener('input', () => {
      // If user changes the key after a successful test, require re-testing.
      if (_state.testPassed) {
        _state.testPassed = false;
        _updateStartButton();
        _clearTestResult();
      }
    });
  }
}

function _handleProviderChange(providerValue) {
  _state.selectedProvider = providerValue;
  _state.testPassed = false;

  // Update card selected state.
  document.querySelectorAll('.onboarding-provider-card').forEach(card => {
    const radio = card.querySelector('input[type="radio"]');
    card.classList.toggle('onboarding-provider-card--selected', radio && radio.checked);
  });

  // Rebuild key section for the new provider.
  const keySection = document.getElementById('onboarding-key-section');
  if (keySection) {
    const providers = [
      { value: 'claude', name: 'Claude', keyHint: 'Get your API key at console.anthropic.com', keyLink: 'https://console.anthropic.com', placeholder: 'sk-ant-…' },
      { value: 'grok',   name: 'Grok',   keyHint: 'Get your API key at console.x.ai',          keyLink: 'https://console.x.ai',          placeholder: 'xai-…'   },
    ];
    _buildKeySection(keySection, providerValue, providers);
  }

  _clearTestResult();
  _updateStartButton();
}

async function _handleTestConnection() {
  const keyInput = document.getElementById('onboarding-api-key');
  const testBtn  = document.getElementById('onboarding-test-btn');
  const apiKey   = keyInput ? keyInput.value.trim() : '';

  if (!apiKey) {
    _showTestResult('error', 'Please enter your API key first.');
    return;
  }

  if (testBtn) {
    testBtn.disabled    = true;
    testBtn.textContent = 'Testing…';
  }
  _showTestResult('loading', 'Connecting to provider…');

  try {
    const res = await fetch('/api/test-provider', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({
        provider: _state.selectedProvider,
        api_key:  apiKey,
      }),
    });

    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }

    const data = await res.json();

    if (data.success) {
      _state.testPassed = true;
      _showTestResult('success', '✓ Connection successful! Your API key works.');
      _updateStartButton();

      // Save the API key to config so it persists.
      await _saveAPIKey(_state.selectedProvider, apiKey);
    } else {
      _state.testPassed = false;
      _showTestResult('error', data.error || 'Connection failed. Please check your API key.');
      _updateStartButton();
    }

  } catch (err) {
    _state.testPassed = false;
    _showTestResult('error', 'Could not reach the server. Please try again.');
    console.error('[onboarding] test-provider error:', err);
    _updateStartButton();
  } finally {
    if (testBtn) {
      testBtn.disabled    = false;
      testBtn.textContent = 'Test Connection';
    }
  }
}

async function _saveAPIKey(providerName, apiKey) {
  try {
    // Fetch current settings first to avoid overwriting other fields.
    const settingsRes = await fetch('/api/settings');
    if (!settingsRes.ok) return;
    const settings = await settingsRes.json();

    // Inject the new API key for the selected provider.
    if (providerName === 'claude') {
      settings.providers.claude.api_key = apiKey;
    } else if (providerName === 'grok') {
      settings.providers.grok.api_key = apiKey;
    }

    // Set the active provider to the one the user just tested.
    settings.provider = providerName;

    await fetch('/api/settings', {
      method:  'PUT',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify(settings),
    });
  } catch (err) {
    // Non-fatal — the key test passed, the user can set it in Settings.
    console.warn('[onboarding] could not save API key to settings:', err);
  }
}

function _handleSkip() {
  _state.skipped = true;
  _dismiss();
}

function _handleStart() {
  _dismiss();
}

function _updateStartButton() {
  const btn = document.getElementById('onboarding-start');
  if (btn) {
    btn.disabled = !_state.testPassed;
  }
}

// ── Test result display ───────────────────────────────────────────

function _showTestResult(type, message) {
  const el = document.getElementById('onboarding-test-result');
  if (!el) return;

  el.className = `onboarding-test-result onboarding-test-result--${type}`;
  el.textContent = message;
}

function _clearTestResult() {
  const el = document.getElementById('onboarding-test-result');
  if (!el) return;
  el.className   = 'onboarding-test-result';
  el.textContent = '';
}

function _showStepError(message) {
  // Find or create an error banner at the top of the card.
  let banner = document.getElementById('onboarding-error-banner');
  if (!banner) {
    banner = document.createElement('div');
    banner.id        = 'onboarding-error-banner';
    banner.className = 'onboarding-error-banner';
    banner.setAttribute('role', 'alert');
    const card = document.getElementById('onboarding-card');
    if (card) card.prepend(banner);
  }
  banner.textContent = message;
}

// ── Utility ───────────────────────────────────────────────────────

function _escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// ── Export ────────────────────────────────────────────────────────

window.Onboarding = Onboarding;
