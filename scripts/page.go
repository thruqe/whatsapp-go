package main

import (
	"fmt"
	"os"
	"path/filepath"

	hb "github.com/Thruqe/htmlbuilder"
)

const pageCSS = `
    :root {
      --color-primary: #000000;
      --color-on-primary: #ffffff;
      --color-ink: #000000;
      --color-body: #5e5e5e;
      --color-mute: #afafaf;
      --color-hairline-mid: #4b4b4b;
      --color-canvas: #ffffff;
      --color-canvas-soft: #efefef;
      --color-canvas-softer: #f3f3f3;
      --color-surface-pressed: #e2e2e2;
      --color-link: #0000ee;
      --color-on-dark: #ffffff;
      --color-black-elevated: #282828;

      --font-sans: 'Geist', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      --font-mono: 'Geist Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, monospace;

      --radius-none: 0px;
      --radius-md: 8px;
      --radius-lg: 12px;
      --radius-xl: 16px;
      --radius-pill: 999px;
      --radius-pill-tab: 36px;
      --radius-full: 9999px;

      --shadow-level-1: 0px 4px 16px 0px rgba(0, 0, 0, 0.08);
      --shadow-level-2: 0px 6px 24px 0px rgba(0, 0, 0, 0.12);
      --shadow-level-3: 0px 2px 8px 0px rgba(0, 0, 0, 0.16);
    }

    *, *::before, *::after {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }

    body {
      font-family: var(--font-sans);
      background-color: var(--color-canvas);
      color: var(--color-ink);
      -webkit-font-smoothing: antialiased;
      letter-spacing: -0.01em;
    }

    .viewport-wrapper {
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background-color: var(--color-canvas);
      padding: 24px;
    }

    .card-frame {
      width: 100%;
      max-width: 440px;
      background-color: var(--color-canvas);
      border-radius: var(--radius-xl);
      padding: 32px;
      box-shadow: var(--shadow-level-2);
      border: 1px solid var(--color-canvas-soft);
      position: relative;
    }

    .progress-bar-track {
      width: 100%;
      height: 3px;
      background-color: var(--color-canvas-soft);
      border-radius: var(--radius-pill);
      overflow: hidden;
      margin-bottom: 24px;
    }

    .progress-bar-fill {
      height: 100%;
      background-color: var(--color-primary);
      transition: width 0.3s cubic-bezier(0.16, 1, 0.3, 1);
    }

    .error-banner {
      background-color: var(--color-canvas-soft);
      border-left: 3px solid var(--color-primary);
      padding: 10px 14px;
      font-size: 13px;
      margin-bottom: 16px;
      display: none;
    }

    .step-slider {
      position: relative;
    }

    .step-body {
      display: none;
      animation: fadeInStep 0.24s cubic-bezier(0.16, 1, 0.3, 1) forwards;
    }

    .step-body.active {
      display: block;
    }

    @keyframes fadeInStep {
      from {
        opacity: 0;
        transform: translateX(12px);
      }
      to {
        opacity: 1;
        transform: translateX(0);
      }
    }

    .header-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }

    .display-sm {
      font-size: 21px;
      font-weight: 600;
      line-height: 28px;
      letter-spacing: -0.02em;
      color: var(--color-ink);
    }

    .body-sm-muted {
      font-size: 14px;
      color: var(--color-body);
      margin-top: 4px;
      margin-bottom: 20px;
    }

    .field-label {
      font-size: 13px;
      font-weight: 500;
      color: var(--color-body);
      margin-bottom: 8px;
    }

    .pill-toggle-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      background-color: var(--color-canvas-soft);
      padding: 4px;
      border-radius: var(--radius-pill-tab);
      gap: 4px;
    }

    .segment-pill {
      background: transparent;
      border: none;
      border-radius: var(--radius-pill-tab);
      padding: 10px 0;
      font-size: 14px;
      font-weight: 500;
      font-family: inherit;
      color: var(--color-ink);
      cursor: pointer;
      transition: background 0.15s ease, box-shadow 0.15s ease;
    }

    .segment-pill.selected {
      background-color: var(--color-canvas);
      box-shadow: var(--shadow-level-3);
    }

    .custom-dropdown-container {
      position: relative;
      width: 100%;
    }

    .custom-dropdown-btn {
      width: 100%;
      background-color: var(--color-canvas-soft);
      border: 1px solid transparent;
      border-radius: var(--radius-md);
      padding: 14px 16px;
      font-size: 15px;
      font-weight: 500;
      font-family: inherit;
      color: var(--color-ink);
      display: flex;
      justify-content: space-between;
      align-items: center;
      cursor: pointer;
      transition: background 0.15s ease, border-color 0.15s ease;
    }

    .custom-dropdown-btn:hover {
      background-color: var(--color-surface-pressed);
    }

    .custom-dropdown-btn.open {
      border-color: var(--color-hairline-mid);
      background-color: var(--color-canvas);
      box-shadow: var(--shadow-level-3);
    }

    .chevron {
      transition: transform 0.2s cubic-bezier(0.16, 1, 0.3, 1);
      color: var(--color-body);
    }

    .chevron.rotated {
      transform: rotate(180deg);
    }

    .custom-dropdown-menu {
      position: absolute;
      top: calc(100% + 6px);
      left: 0;
      width: 100%;
      background-color: var(--color-canvas);
      border-radius: var(--radius-lg);
      box-shadow: var(--shadow-level-2);
      border: 1px solid var(--color-canvas-soft);
      padding: 6px;
      z-index: 100;
      overflow: hidden;
      display: none;
    }

    .custom-dropdown-menu.open {
      display: block;
    }

    .dropdown-item {
      width: 100%;
      border: none;
      background: transparent;
      border-radius: var(--radius-md);
      padding: 10px 12px;
      text-align: left;
      cursor: pointer;
      font-family: inherit;
      transition: background 0.12s ease;
    }

    .dropdown-item:hover, .dropdown-item.active {
      background-color: var(--color-canvas-soft);
    }

    .dropdown-item-title {
      font-size: 14px;
      font-weight: 500;
      color: var(--color-ink);
    }

    .dropdown-item-desc {
      font-size: 12px;
      color: var(--color-body);
      margin-top: 2px;
    }

    .input-row {
      background-color: var(--color-canvas-soft);
      border-radius: var(--radius-md);
      padding: 14px 16px;
      margin-bottom: 16px;
    }

    .form-input {
      width: 100%;
      border: none;
      background: transparent;
      font-size: 15px;
      font-family: inherit;
      color: var(--color-ink);
      outline: none;
    }

    .helper-line {
      margin-top: -6px;
      margin-bottom: 24px;
      display: flex;
      gap: 6px;
      align-items: center;
      font-size: 13px;
    }

    .body-sm {
      color: var(--color-body);
    }

    .link-blue {
      color: var(--color-link);
      text-decoration: underline;
    }

    .preference-box {
      border-radius: var(--radius-lg);
      background-color: var(--color-canvas-soft);
      padding: 6px 16px;
    }

    .switch-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 12px 0;
      cursor: pointer;
    }

    .switch-row:not(:last-child) {
      border-bottom: 1px solid var(--color-surface-pressed);
    }

    .switch-title {
      font-size: 14px;
      font-weight: 500;
    }

    .switch-desc {
      font-size: 12px;
      color: var(--color-body);
    }

    .native-toggle {
      width: 18px;
      height: 18px;
      accent-color: var(--color-primary);
      cursor: pointer;
    }

    .btn-primary {
      background-color: var(--color-primary);
      color: var(--color-on-primary);
      font-size: 15px;
      font-weight: 500;
      font-family: inherit;
      border-radius: var(--radius-pill);
      padding: 12px 20px;
      border: none;
      cursor: pointer;
      width: 100%;
      transition: background-color 0.15s ease;
    }

    .btn-primary:active {
      background-color: var(--color-black-elevated);
    }

    .btn-subtle {
      background-color: var(--color-canvas-soft);
      color: var(--color-ink);
      font-size: 15px;
      font-weight: 500;
      font-family: inherit;
      border-radius: var(--radius-pill);
      padding: 12px 20px;
      border: none;
      cursor: pointer;
      transition: background-color 0.15s ease;
    }

    .btn-subtle:active {
      background-color: var(--color-surface-pressed);
    }

    .button-row {
      display: grid;
      grid-template-columns: 1fr 2fr;
      gap: 12px;
      margin-top: 8px;
    }

    .button-row-stacked {
      display: flex;
      flex-direction: column;
      gap: 10px;
    }

    .tooltip-wrapper {
      position: relative;
      display: inline-flex;
    }

    .tooltip-trigger {
      width: 22px;
      height: 22px;
      border-radius: var(--radius-full);
      background-color: var(--color-canvas-soft);
      border: none;
      font-size: 11px;
      font-weight: 600;
      color: var(--color-body);
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .uber-tooltip {
      position: absolute;
      bottom: calc(100% + 8px);
      right: 0;
      width: 220px;
      background-color: var(--color-ink);
      color: var(--color-on-dark);
      font-size: 12px;
      line-height: 16px;
      padding: 10px 12px;
      border-radius: var(--radius-md);
      box-shadow: var(--shadow-level-3);
      z-index: 120;
      pointer-events: none;
      display: none;
    }

    .tooltip-wrapper:hover .uber-tooltip {
      display: block;
    }

    .skeleton-container {
      background-color: var(--color-canvas-soft);
      border-radius: var(--radius-md);
      padding: 24px 16px;
      display: flex;
      flex-direction: column;
      gap: 12px;
    }

    .skeleton-line {
      height: 12px;
      border-radius: 4px;
    }

    .skeleton-btn {
      height: 44px;
      border-radius: var(--radius-pill);
    }

    .shimmer {
      background: linear-gradient(
        90deg,
        #e2e2e2 0%,
        #ffffff 50%,
        #e2e2e2 100%
      );
      background-size: 250% 100%;
      animation: uberShimmer 1.1s infinite linear;
    }

    @keyframes uberShimmer {
      0% { background-position: -250% 0; }
      100% { background-position: 250% 0; }
    }

    .token-container {
      background-color: var(--color-canvas-soft);
      border-radius: var(--radius-md);
      padding: 16px;
      margin-bottom: 20px;
      word-break: break-all;
      max-height: 140px;
      overflow-y: auto;
    }

    .token-container code {
      font-family: var(--font-mono);
      font-size: 12px;
      line-height: 18px;
      color: var(--color-ink);
    }
`

const pageJS = `
  document.addEventListener('DOMContentLoaded', () => {
    const config = {
      phone: '',
      business: false,
      auth: 'qr',
      client: 'default',
      db: '',
      autoupdate: true,
      verbose: false
    };

    let currentStep = 1;
    let generatedToken = '';
    const totalSteps = 5;

    const progressFill = document.getElementById('progress-fill');
    const errorBanner = document.getElementById('error-banner');

    function showError(msg) {
      if (!msg) {
        errorBanner.style.display = 'none';
        errorBanner.textContent = '';
      } else {
        errorBanner.textContent = msg;
        errorBanner.style.display = 'block';
      }
    }

    function setStep(newStep) {
      showError('');
      document.querySelectorAll('.step-body').forEach(el => el.classList.remove('active'));
      const activeEl = document.getElementById('step-' + newStep);
      if (activeEl) activeEl.classList.add('active');
      currentStep = newStep;
      progressFill.style.width = ((currentStep / totalSteps) * 100) + '%';
    }

    const typeRegular = document.getElementById('type-regular');
    const typeBusiness = document.getElementById('type-business');
    typeRegular.addEventListener('click', () => {
      config.business = false;
      typeRegular.classList.add('selected');
      typeBusiness.classList.remove('selected');
    });
    typeBusiness.addEventListener('click', () => {
      config.business = true;
      typeBusiness.classList.add('selected');
      typeRegular.classList.remove('selected');
    });

    const dropdownToggle = document.getElementById('dropdown-toggle');
    const dropdownMenu = document.getElementById('dropdown-menu');
    const dropdownChevron = document.getElementById('dropdown-chevron');
    const selectedLabel = document.getElementById('selected-client-label');

    dropdownToggle.addEventListener('click', (e) => {
      e.stopPropagation();
      const isOpen = dropdownMenu.classList.toggle('open');
      dropdownToggle.classList.toggle('open', isOpen);
      dropdownChevron.classList.toggle('rotated', isOpen);
    });

    document.querySelectorAll('.dropdown-item').forEach(item => {
      item.addEventListener('click', (e) => {
        e.stopPropagation();
        const val = item.getAttribute('data-val');
        config.client = val;
        document.querySelectorAll('.dropdown-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        selectedLabel.textContent = item.querySelector('.dropdown-item-title').textContent;
        dropdownMenu.classList.remove('open');
        dropdownToggle.classList.remove('open');
        dropdownChevron.classList.remove('rotated');
      });
    });

    document.addEventListener('click', () => {
      dropdownMenu.classList.remove('open');
      dropdownToggle.classList.remove('open');
      dropdownChevron.classList.remove('rotated');
    });

    document.getElementById('btn-step1-next').addEventListener('click', () => setStep(2));

    const authQr = document.getElementById('auth-qr');
    const authPair = document.getElementById('auth-pair');
    const phoneLabel = document.getElementById('phone-label');
    const inputPhone = document.getElementById('input-phone');

    authQr.addEventListener('click', () => {
      config.auth = 'qr';
      authQr.classList.add('selected');
      authPair.classList.remove('selected');
      phoneLabel.textContent = 'Phone number (optional for labels)';
    });
    authPair.addEventListener('click', () => {
      config.auth = 'pair';
      authPair.classList.add('selected');
      authQr.classList.remove('selected');
      phoneLabel.textContent = 'Phone number (required for pairing)';
    });

    function nextFromStep2() {
      showError('');
      const cleanPhone = inputPhone.value.trim().replace(/[\s()+-]/g, '');
      if (config.auth === 'pair') {
        if (!cleanPhone) {
          showError('Please enter your phone number with your country code to get a pairing code.');
          return;
        }
        if (!/^\+?\d{8,15}$/.test(cleanPhone)) {
          showError('Please enter a valid phone number (e.g., +2348012345678).');
          return;
        }
      }
      config.phone = cleanPhone;
      setStep(3);
    }

    document.getElementById('btn-step2-next').addEventListener('click', nextFromStep2);
    document.getElementById('btn-step2-back').addEventListener('click', () => setStep(1));
    inputPhone.addEventListener('keydown', (e) => { if (e.key === 'Enter') nextFromStep2(); });

    const inputDb = document.getElementById('input-db');
    function isValidPostgresUrl(url) {
      const trimmed = url.trim();
      if (!trimmed) return false;
      const pattern = /^(postgres|postgresql):\/\/(?:([^:]+)(?::([^@]*))?@)?([a-zA-Z0-9.-]+)(?::(\d+))?\/([a-zA-Z0-9_-]+)(?:\?.*)?$/;
      return pattern.test(trimmed);
    }

    document.getElementById('btn-step3-save').addEventListener('click', () => {
      showError('');
      const trimmed = inputDb.value.trim();
      if (!trimmed) {
        showError('Please enter a database URL or click "Skip this step".');
        return;
      }
      if (!isValidPostgresUrl(trimmed)) {
        showError('Please enter a valid PostgreSQL URL (e.g. postgresql://user:pass@host:5432/dbname).');
        return;
      }
      config.db = trimmed;
      setStep(4);
    });

    document.getElementById('btn-step3-skip').addEventListener('click', () => {
      config.db = '';
      setStep(4);
    });

    inputDb.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        document.getElementById('btn-step3-save').click();
      }
    });

    const checkAutoupdate = document.getElementById('check-autoupdate');
    const checkVerbose = document.getElementById('check-verbose');

    document.getElementById('btn-step4-back').addEventListener('click', () => setStep(3));
    document.getElementById('btn-step4-generate').addEventListener('click', async () => {
      config.autoupdate = checkAutoupdate.checked;
      config.verbose = checkVerbose.checked;
      setStep(5);

      const skeleton = document.getElementById('result-skeleton');
      const content = document.getElementById('result-content');
      skeleton.style.display = 'block';
      content.style.display = 'none';

      try {
        const [token] = await Promise.all([
          generateSessionToken(config),
          new Promise(r => setTimeout(r, 800))
        ]);
        generatedToken = token;
        document.getElementById('token-display').textContent = token;
        skeleton.style.display = 'none';
        content.style.display = 'block';
      } catch (err) {
        showError('Failed to generate session ID. Please check your inputs.');
        skeleton.style.display = 'none';
      }
    });

    const btnCopy = document.getElementById('btn-copy-token');
    btnCopy.addEventListener('click', () => {
      if (!generatedToken) return;
      navigator.clipboard.writeText(generatedToken);
      btnCopy.textContent = 'Copied to Clipboard!';
      setTimeout(() => { btnCopy.textContent = 'Copy Session ID'; }, 2000);
    });

    document.getElementById('btn-create-another').addEventListener('click', () => {
      config.phone = '';
      config.business = false;
      config.auth = 'qr';
      config.client = 'default';
      config.db = '';
      config.autoupdate = true;
      config.verbose = false;

      typeRegular.classList.add('selected');
      typeBusiness.classList.remove('selected');
      authQr.classList.add('selected');
      authPair.classList.remove('selected');
      phoneLabel.textContent = 'Phone number (optional for labels)';
      inputPhone.value = '';
      inputDb.value = '';
      checkAutoupdate.checked = true;
      checkVerbose.checked = false;
      selectedLabel.textContent = 'Default (Web Browser)';
      document.querySelectorAll('.dropdown-item').forEach(i => {
        i.classList.toggle('active', i.getAttribute('data-val') === 'default');
      });

      generatedToken = '';
      setStep(1);
    });

    const RAW_KEY = new TextEncoder().encode("ThruqeCustomWABotMasterKey_2026!");

    async function getCryptoKey() {
      return await crypto.subtle.importKey(
        "raw",
        RAW_KEY,
        { name: "AES-GCM" },
        false,
        ["encrypt"]
      );
    }

    async function compress(str) {
      const stream = new Blob([str]).stream().pipeThrough(new CompressionStream("deflate-raw"));
      const buffer = await new Response(stream).arrayBuffer();
      return new Uint8Array(buffer);
    }

    function toBase64RawUrl(bytes) {
      let binary = "";
      for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i]);
      }
      return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    }

    async function generateSessionToken(cfg) {
      const jsonStr = JSON.stringify(cfg);
      const compressed = await compress(jsonStr);

      const key = await getCryptoKey();
      const nonce = crypto.getRandomValues(new Uint8Array(12));

      const encryptedBuffer = await crypto.subtle.encrypt(
        { name: "AES-GCM", iv: nonce },
        key,
        compressed
      );

      const ciphertext = new Uint8Array(encryptedBuffer);
      const combined = new Uint8Array(nonce.length + ciphertext.length);
      combined.set(nonce, 0);
      combined.set(ciphertext, nonce.length);

      return "SE_ID:" + toBase64RawUrl(combined);
    }
  });
`

// BuildPage constructs the Session ID generator HTML using htmlbuilder.
func BuildPage() *hb.Document {
	doc := hb.New()
	doc.Lang("en")
	doc.Title("WhatsRook — Session ID Generator")
	doc.Icon("https://pub-687f7ee583cf4f339cab34d2b3a3871d.r2.dev/logo.svg", "image/svg+xml")
	doc.Meta(map[string]string{"charset": "utf-8"})
	doc.Meta(map[string]string{"name": "viewport", "content": "width=device-width, initial-scale=1"})
	doc.Link(map[string]string{"rel": "preconnect", "href": "https://fonts.googleapis.com"})
	doc.Link(map[string]string{"rel": "preconnect", "href": "https://fonts.gstatic.com", "crossorigin": ""})
	doc.Link(map[string]string{"rel": "stylesheet", "href": "https://fonts.googleapis.com/css2?family=Geist+Mono:wght@400;500&family=Geist:wght@400;500;600;700&display=swap"})

	doc.StyleBlock(pageCSS)

	// Build Body tree
	step1 := hb.Div(
		hb.Div(
			hb.H2("WhatsApp Account").Class("display-sm"),
			hb.Div(
				hb.Button(hb.El("span").Text("?")).Class("tooltip-trigger").Attr("type", "button").Attr("aria-label", "Help"),
				hb.Div().Class("uber-tooltip").Text("Choose whether you are connecting a personal WhatsApp number or a business account."),
			).Class("tooltip-wrapper"),
		).Class("header-row"),
		hb.P("Choose your WhatsApp type and device profile").Class("body-sm-muted"),
		hb.Div().Class("field-label").Text("Account type"),
		hb.Div(
			hb.Button(hb.El("span").Text("Regular WhatsApp")).Class("segment-pill", "selected").Attr("id", "type-regular").Attr("type", "button"),
			hb.Button(hb.El("span").Text("WhatsApp Business")).Class("segment-pill").Attr("id", "type-business").Attr("type", "button"),
		).Class("pill-toggle-grid"),
		hb.Div().Class("field-label").Attr("style", "margin-top: 20px;").Text("Device appearance"),
		hb.Div(
			hb.Button(
				hb.Span("Default (Web Browser)").Attr("id", "selected-client-label"),
				hb.El("span").Raw(`<svg class="chevron" id="dropdown-chevron" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M3.2 5.5l4.8 4.8 4.8-4.8.7.7-5.5 5.5-5.5-5.5.7-.7z"/></svg>`),
			).Class("custom-dropdown-btn").Attr("id", "dropdown-toggle").Attr("type", "button"),
			hb.Div(
				hb.Button(
					hb.Div().Class("dropdown-item-title").Text("Default (Web Browser)"),
					hb.Div().Class("dropdown-item-desc").Text("Standard browser session"),
				).Class("dropdown-item", "active").Attr("type", "button").Attr("data-val", "default"),
				hb.Button(
					hb.Div().Class("dropdown-item-title").Text("Android Phone"),
					hb.Div().Class("dropdown-item-desc").Text("Simulates official Android app"),
				).Class("dropdown-item").Attr("type", "button").Attr("data-val", "android"),
				hb.Button(
					hb.Div().Class("dropdown-item-title").Text("iPhone / iPad"),
					hb.Div().Class("dropdown-item-desc").Text("Simulates official iOS app"),
				).Class("dropdown-item").Attr("type", "button").Attr("data-val", "ios"),
			).Class("custom-dropdown-menu").Attr("id", "dropdown-menu"),
		).Class("custom-dropdown-container"),
		hb.Button(hb.El("span").Text("Continue")).Class("btn-primary").Attr("id", "btn-step1-next").Attr("style", "margin-top: 24px;"),
	).Class("step-body", "active").Attr("id", "step-1")

	step2 := hb.Div(
		hb.Div(
			hb.H2("Link Your Account").Class("display-sm"),
			hb.Div(
				hb.Button(hb.El("span").Text("?")).Class("tooltip-trigger").Attr("type", "button").Attr("aria-label", "Help"),
				hb.Div().Class("uber-tooltip").Text("With QR code, your bot shows a code on terminal to scan. With Pairing code, it gives you an 8-digit number to enter directly into WhatsApp."),
			).Class("tooltip-wrapper"),
		).Class("header-row"),
		hb.P("Select how you want to link your bot").Class("body-sm-muted"),
		hb.Div(
			hb.Button(hb.El("span").Text("Scan QR Code")).Class("segment-pill", "selected").Attr("id", "auth-qr").Attr("type", "button"),
			hb.Button(hb.El("span").Text("Use Pairing Code")).Class("segment-pill").Attr("id", "auth-pair").Attr("type", "button"),
		).Class("pill-toggle-grid"),
		hb.Div().Class("field-label").Attr("id", "phone-label").Attr("style", "margin-top: 20px;").Text("Phone number (optional for labels)"),
		hb.Div(
			hb.Input().Attr("type", "tel").Attr("id", "input-phone").Attr("placeholder", "+234 801 234 5678").Class("form-input"),
		).Class("input-row"),
		hb.Div(
			hb.Button(hb.El("span").Text("Back")).Class("btn-subtle").Attr("id", "btn-step2-back"),
			hb.Button(hb.El("span").Text("Next")).Class("btn-primary").Attr("id", "btn-step2-next"),
		).Class("button-row"),
	).Class("step-body").Attr("id", "step-2")

	step3 := hb.Div(
		hb.Div(
			hb.H2("Database Storage").Class("display-sm"),
			hb.Div(
				hb.Button(hb.El("span").Text("?")).Class("tooltip-trigger").Attr("type", "button").Attr("aria-label", "Help"),
				hb.Div().Class("uber-tooltip").Text("Enter a PostgreSQL connection string to persist your contacts and messages in the cloud. If you skip, the bot automatically uses local file storage."),
			).Class("tooltip-wrapper"),
		).Class("header-row"),
		hb.P("Store your chats and sessions in the cloud").Class("body-sm-muted"),
		hb.Div().Class("field-label").Text("PostgreSQL / Supabase URL"),
		hb.Div(
			hb.Input().Attr("type", "text").Attr("id", "input-db").Attr("placeholder", "postgresql://postgres:pass@db.example.com:5432/postgres").Class("form-input"),
		).Class("input-row"),
		hb.Div(
			hb.Span("Need a free database?").Class("body-sm"),
			hb.A("Get one on Supabase").Attr("href", "https://supabase.com").Attr("target", "_blank").Attr("rel", "noreferrer").Class("link-blue"),
		).Class("helper-line"),
		hb.Div(
			hb.Button(hb.El("span").Text("Save Database")).Class("btn-primary").Attr("id", "btn-step3-save"),
			hb.Button(hb.El("span").Text("Skip this step (Use local storage)")).Class("btn-subtle").Attr("id", "btn-step3-skip"),
		).Class("button-row-stacked"),
	).Class("step-body").Attr("id", "step-3")

	step4 := hb.Div(
		hb.Div(
			hb.H2("Preferences").Class("display-sm"),
			hb.Div(
				hb.Button(hb.El("span").Text("?")).Class("tooltip-trigger").Attr("type", "button").Attr("aria-label", "Help"),
				hb.Div().Class("uber-tooltip").Text("Keep automatic updates turned on to ensure security patches and protocol fixes are applied seamlessly."),
			).Class("tooltip-wrapper"),
		).Class("header-row"),
		hb.P("Configure basic bot behavior").Class("body-sm-muted"),
		hb.Div(
			hb.El("label").Class("switch-row").Child(
				hb.Div(
					hb.Div().Class("switch-title").Text("Auto-updates"),
					hb.Div().Class("switch-desc").Text("Check for bot updates on startup"),
				),
				hb.Input().Attr("type", "checkbox").Attr("id", "check-autoupdate").Attr("checked", "").Class("native-toggle"),
			),
			hb.El("label").Class("switch-row").Child(
				hb.Div(
					hb.Div().Class("switch-title").Text("Detailed logs"),
					hb.Div().Class("switch-desc").Text("Show connection events in terminal"),
				),
				hb.Input().Attr("type", "checkbox").Attr("id", "check-verbose").Class("native-toggle"),
			),
		).Class("preference-box"),
		hb.Div(
			hb.Button(hb.El("span").Text("Back")).Class("btn-subtle").Attr("id", "btn-step4-back"),
			hb.Button(hb.El("span").Text("Generate ID")).Class("btn-primary").Attr("id", "btn-step4-generate"),
		).Class("button-row").Attr("style", "margin-top: 24px;"),
	).Class("step-body").Attr("id", "step-4")

	step5 := hb.Div(
		hb.H2("Your Session ID").Class("display-sm"),
		hb.P("Copy and paste this into your bot configuration").Class("body-sm-muted"),
		hb.Div(
			hb.Div(
				hb.Div().Class("skeleton-line", "shimmer").Attr("style", "width: 100%;"),
				hb.Div().Class("skeleton-line", "shimmer").Attr("style", "width: 82%;"),
				hb.Div().Class("skeleton-line", "shimmer").Attr("style", "width: 50%;"),
			).Class("skeleton-container").Attr("aria-busy", "true"),
			hb.Div(
				hb.Div().Class("skeleton-btn", "shimmer"),
				hb.Div().Class("skeleton-btn", "shimmer"),
			).Class("button-row").Attr("style", "margin-top: 20px;"),
		).Attr("id", "result-skeleton"),
		hb.Div(
			hb.Div(
				hb.Code("").Attr("id", "token-display"),
			).Class("token-container"),
			hb.Div(
				hb.Button(hb.El("span").Text("Create Another")).Class("btn-subtle").Attr("id", "btn-create-another"),
				hb.Button(hb.El("span").Text("Copy Session ID")).Class("btn-primary").Attr("id", "btn-copy-token"),
			).Class("button-row"),
		).Attr("id", "result-content").Attr("style", "display: none;"),
	).Class("step-body").Attr("id", "step-5")

	cardFrame := hb.Div(
		hb.Div(
			hb.Div().Class("progress-bar-fill").Attr("id", "progress-fill").Attr("style", "width: 20%;"),
		).Class("progress-bar-track"),
		hb.Div().Class("error-banner").Attr("id", "error-banner"),
		hb.Div(
			step1,
			step2,
			step3,
			step4,
			step5,
		).Class("step-slider"),
	).Class("card-frame")

	doc.Body().Child(
		hb.Div(cardFrame).Class("viewport-wrapper"),
	)

	doc.Script(pageJS)
	return doc
}

// GenerateSite outputs the generated index.html, robots.txt, and .nojekyll to outDir.
func GenerateSite(outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	doc := BuildPage()
	htmlPath := filepath.Join(outDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(doc.String()), 0644); err != nil {
		return fmt.Errorf("failed to write index.html: %w", err)
	}

	robotsPath := filepath.Join(outDir, "robots.txt")
	robotsContent := "User-agent: *\nAllow: /\n"
	if err := os.WriteFile(robotsPath, []byte(robotsContent), 0644); err != nil {
		return fmt.Errorf("failed to write robots.txt: %w", err)
	}

	nojekyllPath := filepath.Join(outDir, ".nojekyll")
	if err := os.WriteFile(nojekyllPath, []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to write .nojekyll: %w", err)
	}

	return nil
}

func runPage(args []string) error {
	outDir := "_site"
	if len(args) > 0 && args[0] != "" {
		outDir = args[0]
	}
	fmt.Printf("Building static GitHub Pages site using htmlbuilder in %s...\n", outDir)
	if err := GenerateSite(outDir); err != nil {
		return err
	}
	fmt.Println("Static site built successfully!")
	return nil
}
