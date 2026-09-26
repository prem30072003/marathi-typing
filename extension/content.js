/*
 * Content script: turns keystrokes in text fields into Marathi.
 *
 * Works like a phone keyboard's transliteration mode. Letters you type are
 * held in a small suggestion box (they are not typed into the page yet).
 *   Space        insert the highlighted word + space
 *   Enter / Tab  insert the highlighted word
 *   1-7 / click  pick a suggestion
 *   ↑ ↓          move the highlight
 *   Esc          keep what you typed in English letters
 *   Alt+M        turn Marathi typing on/off
 */
(() => {
  if (window.__mrXlitLoaded) return;
  window.__mrXlitLoaded = true;

  const Eng = self.MarathiEngine;
  const docsFrame = (() => {
    try { return !!(window.frameElement && window.frameElement.classList.contains('docs-texteventtarget-iframe')); }
    catch (e) { return false; }
  })();
  const siteHost = (() => { try { return window.top.location.hostname; } catch (e) { return location.hostname; } })();

  let enabled = true;
  let siteOff = false;
  let opts = { keepEnglish: false, digits: false, indicator: true };
  chrome.storage.local.get(['enabled', 'disabledSites', 'opts']).then(({ enabled: en = true, disabledSites = [], opts: o }) => {
    enabled = en; siteOff = disabledSites.includes(siteHost); opts = Object.assign(opts, o || {});
    updateChip();
  }).catch(() => {});
  chrome.storage.onChanged.addListener((c) => {
    if (c.enabled) {
      enabled = c.enabled.newValue !== false;
      if (!enabled) reset();
      if (document.hasFocus() && isEditable(deepActive())) toast(enabled ? 'मराठी चालू · Marathi ON' : 'Marathi OFF · English');
    }
    if (c.disabledSites) { siteOff = (c.disabledSites.newValue || []).includes(siteHost); if (siteOff) reset(); }
    if (c.opts) opts = Object.assign({ keepEnglish: false, digits: false, indicator: true }, c.opts.newValue || {});
    updateChip();
  });

  const DIGITS = '०१२३४५६७८९';

  // ------------------------------------------------------------------ state
  const S = { buf: '', list: [], nComp: 0, listFor: '', sel: 0, picked: false, el: null, req: 0, timer: 0 };
  // the previously typed word in the same field, used as context for suggestions
  const ctx = { el: null, word: '' };
  function prevFor(el) { return ctx.el === el ? ctx.word : ''; }
  let chain = Promise.resolve();

  function reset() {
    S.buf = ''; S.list = []; S.nComp = 0; S.listFor = ''; S.sel = 0; S.picked = false; S.el = null;
    clearTimeout(S.timer);
    hide();
  }

  function deepActive() {
    let a = document.activeElement;
    while (a && a.shadowRoot && a.shadowRoot.activeElement) a = a.shadowRoot.activeElement;
    return a;
  }

  function isEditable(t) {
    if (!t || t.nodeType !== 1) return false;
    if (t.tagName === 'TEXTAREA') return !t.readOnly && !t.disabled;
    if (t.tagName === 'INPUT') {
      const ty = (t.getAttribute('type') || 'text').toLowerCase();
      return (ty === 'text' || ty === 'search') && !t.readOnly && !t.disabled;
    }
    return !!t.isContentEditable;
  }

  // -> {list, nComp}: suggestions followed by nComp word completions
  function ask(input, el) {
    const id = ++S.req;
    const fallback = { list: [Eng.rulesToDev(input)], nComp: 0 };
    return chrome.runtime.sendMessage({ type: 'suggest', id, input, prev: prevFor(el) })
      .then(r => (r && r.list) ? { list: r.list.concat(r.comp || []), nComp: (r.comp || []).length } : fallback)
      .catch(() => fallback);
  }

  function refresh() {
    const input = S.buf;
    clearTimeout(S.timer);
    // show a rule-based preview only if the lexicon is slow (first use after idle)
    S.timer = setTimeout(() => {
      if (S.buf === input && S.listFor !== input) { S.list = [Eng.rulesToDev(input)]; S.nComp = 0; render(); }
    }, 90);
    render();
    ask(input, S.el).then(r => {
      if (S.buf !== input) return;
      S.list = r.list; S.nComp = r.nComp; S.listFor = input;
      if (S.sel >= S.list.length) S.sel = 0;
      render();
    });
  }

  function commit(suffix, raw) {
    const el = S.el, input = S.buf, sel = S.sel, picked = S.picked, prev = prevFor(el);
    const ready = S.listFor === input ? Promise.resolve({ list: S.list }) : ask(input, el);
    reset();
    chain = chain.then(() => ready).then(({ list }) => {
      const word = raw ? input : (list[sel] || list[0] || Eng.rulesToDev(input));
      insert(el, word + suffix);
      if (!raw) chrome.runtime.sendMessage({ type: 'learn', input, word, prev, picked: picked && sel > 0 }).catch(() => {});
      // context for the next word: only within a sentence
      ctx.el = el; ctx.word = (!raw && (suffix === ' ' || suffix === '')) ? word : '';
    });
  }

  function typeChar(el, ch) {
    chain = chain.then(() => { insert(el, ch); ctx.word = ''; });
  }

  // --------------------------------------------------------------- insertion
  function insert(el, text) {
    if (!el || !text) return;
    if (docsFrame) { paste(el, text); return; }
    const focused = deepActive() === el;
    if (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') {
      let ok = false;
      if (focused) { try { ok = document.execCommand('insertText', false, text); } catch (e) { ok = false; } }
      if (!ok) {   // never steal focus back if the user already moved on
        const a = el.selectionStart, b = el.selectionEnd;
        el.setRangeText(text, a, b, 'end');
        el.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }));
      }
      return;
    }
    if (focused || el.contains(deepActive())) {
      let ok = false;
      try { ok = document.execCommand('insertText', false, text); } catch (e) { ok = false; }
      if (!ok) paste(el, text);
    }
  }

  function paste(el, text) {
    const dt = new DataTransfer();
    dt.setData('text/plain', text);
    el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
  }

  // ------------------------------------------------------------------ keys
  function stop(e) { e.preventDefault(); e.stopImmediatePropagation(); }

  function onKeyDown(e) {
    if (!e.isTrusted || e.isComposing || e.keyCode === 229) return;

    // Alt+M also toggles from inside the page (in case the browser shortcut is taken)
    if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.code === 'KeyM') {
      stop(e);
      if (S.buf) commit('', false);
      chrome.storage.local.set({ enabled: !enabled }).catch(() => {});
      return;
    }
    if (!enabled || siteOff) { if (S.buf) reset(); return; }

    const t = e.composedPath ? e.composedPath()[0] : e.target;
    const el = isEditable(t) ? t : (isEditable(deepActive()) ? deepActive() : null);
    if (!el) { if (S.buf) reset(); return; }
    if (S.buf && el !== S.el) reset();

    const k = e.key;
    if (e.ctrlKey || e.metaKey || e.altKey) { if (S.buf) commit('', false); return; }

    // Marathi digits option: 0-9 -> ०-९ (when no word is being typed)
    if (!S.buf && opts.digits && /^[0-9]$/.test(k)) { stop(e); typeChar(el, DIGITS[+k]); return; }
    if (!S.buf) { if (k === 'Enter' || k.length === 1 && /[.?!]/.test(k)) ctx.word = ''; }

    if (k.length === 1 && /[A-Za-z]/.test(k)) {
      stop(e);
      S.el = el; S.buf += k; S.sel = 0; S.picked = false;
      refresh();
      return;
    }
    if (!S.buf) return;

    switch (k) {
      case 'Backspace':
        stop(e); S.buf = S.buf.slice(0, -1); S.sel = 0;
        if (S.buf) refresh(); else reset();
        return;
      case 'Escape': stop(e); commit('', true); return;
      case ' ': stop(e); commit(' ', false); return;
      case 'Enter': case 'Tab': stop(e); commit('', false); return;
      case 'ArrowDown': case 'ArrowUp': {  // eslint-disable-line no-fallthrough
        stop(e);
        const n = S.list.length || 1;
        S.sel = (S.sel + (k === 'ArrowDown' ? 1 : n - 1)) % n; S.picked = true; render();
        return;
      }
    }
    if (/^[1-9]$/.test(k) && +k <= S.list.length) {
      stop(e); S.sel = +k - 1; S.picked = true; commit('', false); return;
    }
    if (k.length === 1) { stop(e); commit(k, false); return; }   // punctuation, digits
    commit('', false);                                           // arrows, Home, ... pass through
  }

  window.addEventListener('keydown', onKeyDown, true);
  let warmed = 0;
  window.addEventListener('focusin', (e) => {
    if (!enabled || siteOff || Date.now() - warmed < 20000) return;
    const t = e.composedPath ? e.composedPath()[0] : e.target;
    if (isEditable(t)) { warmed = Date.now(); chrome.runtime.sendMessage({ type: 'warm' }).catch(() => {}); }
  }, true);
  window.addEventListener('mousedown', (e) => {
    if (host && e.composedPath().includes(host)) return;
    ctx.word = '';
    if (!S.buf) return;
    commit('', false);
  }, true);
  window.addEventListener('focusout', (e) => {
    if (!S.buf || e.target !== S.el) return;
    setTimeout(() => { if (S.buf && deepActive() !== S.el && !docsFrame) reset(); }, 0);
  }, true);

  // --------------------------------------------------------------- popup UI
  let host = null, root = null;
  const uiDoc = (() => { if (docsFrame) { try { return window.parent.document; } catch (e) { } } return document; })();

  const CSS = `
    :host { all: initial; }
    .box { position: fixed; z-index: 2147483647; min-width: 150px; max-width: 360px;
      background: #fff; color: #1c1917; border: 1px solid #d6d3d1; border-radius: 10px;
      box-shadow: 0 8px 24px rgba(0,0,0,.18); font: 14px/1.35 system-ui, -apple-system, "Segoe UI", sans-serif;
      overflow: hidden; }
    .typed { padding: 6px 10px; font-size: 13px; color: #9a3412; background: #fff7ed; border-bottom: 1px solid #fed7aa;
      font-family: ui-monospace, Consolas, monospace; letter-spacing: .02em; }
    ol { list-style: none; margin: 0; padding: 4px; }
    li { display: flex; gap: 8px; align-items: baseline; padding: 4px 8px; border-radius: 6px; cursor: pointer;
      font-family: "Nirmala UI", "Kohinoor Devanagari", "Noto Sans Devanagari", "Mangal", system-ui, sans-serif; font-size: 18px; }
    li b { font: 600 11px system-ui, sans-serif; color: #a8a29e; min-width: 10px; }
    li.sel { background: #ffedd5; }
    li.sel b { color: #c2410c; }
    li.comp { color: #57534e; } li.comp i { font: 12px system-ui, sans-serif; color: #a8a29e; margin-left: auto; }
    li.comp:first-of-type, li:not(.comp) + li.comp { border-top: 1px dashed #e7e5e4; }
    .chip { position: fixed; z-index: 2147483646; width: 22px; height: 22px; border-radius: 6px; display: flex;
      align-items: center; justify-content: center; font: 700 13px "Nirmala UI", "Noto Sans Devanagari", system-ui, sans-serif;
      color: #fff; background: #c2410c; opacity: .85; cursor: pointer; box-shadow: 0 1px 4px rgba(0,0,0,.25); user-select: none; }
    .chip.off { background: #78716c; font: 700 10px system-ui, sans-serif; }
    .chip:hover { opacity: 1; }
    .hint { padding: 4px 10px 6px; font-size: 11px; color: #78716c; border-top: 1px solid #f5f5f4; }
    @media (prefers-color-scheme: dark) {
      .box { background: #1c1917; color: #fafaf9; border-color: #44403c; }
      .typed { background: #292524; color: #fdba74; border-color: #44403c; }
      li.sel { background: #431407; } .hint { color: #a8a29e; border-color: #292524; }
    }
    .toast { position: fixed; z-index: 2147483647; right: 16px; bottom: 16px; padding: 8px 12px; border-radius: 8px;
      background: #1c1917; color: #fff; font: 13px system-ui, sans-serif; box-shadow: 0 6px 18px rgba(0,0,0,.25); }`;

  function ensureUI() {
    if (host && host.isConnected) return;
    host = uiDoc.createElement('mr-xlit-popup');
    root = host.attachShadow({ mode: 'open' });
    const st = uiDoc.createElement('style'); st.textContent = CSS; root.appendChild(st);
    (uiDoc.body || uiDoc.documentElement).appendChild(host);
    root.addEventListener('mousedown', (e) => {
      const li = e.target.closest && e.target.closest('li');
      e.preventDefault(); e.stopPropagation();
      if (e.target.classList && e.target.classList.contains('chip')) {
        if (S.buf) commit('', false);
        chrome.storage.local.set({ enabled: !enabled }).catch(() => {});
        return;
      }
      if (li && S.buf) { S.sel = +li.dataset.i; S.picked = true; commit('', false); }
    });
  }

  function hide() { if (root) { const b = root.querySelector('.box'); if (b) b.remove(); } }

  function esc(s) { return s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); }

  function render() {
    if (!S.buf) { hide(); return; }
    ensureUI();
    let box = root.querySelector('.box');
    if (!box) { box = uiDoc.createElement('div'); box.className = 'box'; root.appendChild(box); }
    const list = S.list.length ? S.list : [Eng.rulesToDev(S.buf)];
    box.innerHTML = `<div class="typed">${esc(S.buf)}</div><ol>` +
      list.map((w, i) => {
        const comp = i >= list.length - S.nComp && S.list.length === list.length;
        return `<li data-i="${i}" class="${i === S.sel ? 'sel' : ''}${comp ? ' comp' : ''}"><b>${i + 1}</b><span>${esc(w)}</span>${comp ? '<i>…</i>' : ''}</li>`;
      }).join('') +
      `</ol><div class="hint">Space ✓ · 1–${list.length} choose · Esc English</div>`;
    place(box);
  }

  function place(box) {
    const r = caretRect();
    const vw = uiDoc.documentElement.clientWidth, vh = uiDoc.documentElement.clientHeight;
    const bw = box.offsetWidth, bh = box.offsetHeight;
    let x = Math.min(Math.max(8, r.left), vw - bw - 8);
    let y = r.bottom + 6;
    if (y + bh > vh - 8) y = Math.max(8, r.top - bh - 6);
    box.style.left = x + 'px'; box.style.top = y + 'px';
  }

  function caretRect() {
    const el = S.el;
    const fallback = () => { const r = el.getBoundingClientRect(); return { left: r.left, top: r.top, bottom: Math.min(r.bottom, r.top + 28) }; };
    try {
      if (docsFrame) {
        const c = uiDoc.querySelector('.kix-cursor-caret') || uiDoc.querySelector('.kix-cursor');
        if (c) { const r = c.getBoundingClientRect(); if (r.height) return r; }
        return { left: 80, top: 80, bottom: 110 };
      }
      if (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') return inputCaret(el);
      const sel = el.getRootNode().getSelection ? el.getRootNode().getSelection() : getSelection();
      if (sel && sel.rangeCount) {
        const rg = sel.getRangeAt(0).cloneRange(); rg.collapse(false);
        let rs = rg.getClientRects();
        if (rs.length) return rs[rs.length - 1];
        const m = document.createElement('span'); m.textContent = '​'; rg.insertNode(m);
        const r = m.getBoundingClientRect(); m.remove();
        if (r.height) return r;
      }
    } catch (e) { }
    return fallback();
  }

  function inputCaret(el) {
    const cs = getComputedStyle(el), d = document.createElement('div');
    ['boxSizing', 'width', 'height', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
      'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'fontStyle', 'fontVariant', 'fontWeight',
      'fontStretch', 'fontSize', 'lineHeight', 'fontFamily', 'textAlign', 'textTransform', 'textIndent',
      'letterSpacing', 'wordSpacing', 'tabSize'].forEach(p => { d.style[p] = cs[p]; });
    Object.assign(d.style, { position: 'absolute', visibility: 'hidden', top: '0', left: '-9999px', overflow: 'hidden',
      whiteSpace: el.tagName === 'INPUT' ? 'pre' : 'pre-wrap', wordWrap: 'break-word', borderStyle: 'solid' });
    const pos = el.selectionEnd == null ? el.value.length : el.selectionEnd;
    d.textContent = el.value.slice(0, pos);
    const sp = document.createElement('span'); sp.textContent = '​'; d.appendChild(sp);
    (document.body || document.documentElement).appendChild(d);
    const r = el.getBoundingClientRect();
    const lh = sp.offsetHeight || parseFloat(cs.fontSize) * 1.3;
    const left = r.left + parseFloat(cs.borderLeftWidth) + sp.offsetLeft - el.scrollLeft;
    const top = r.top + parseFloat(cs.borderTopWidth) + sp.offsetTop - el.scrollTop;
    d.remove();
    return { left: Math.min(left, r.right), top, bottom: Math.min(top + lh, r.bottom) };
  }

  // small म / EN chip in the corner of the focused text field
  let chipEl = null, chipRaf = 0;
  function updateChip() {
    cancelAnimationFrame(chipRaf);
    chipRaf = requestAnimationFrame(() => {
      const el = deepActive();
      const show = opts.indicator && !siteOff && document.hasFocus() && isEditable(el);
      if (!show) { if (chipEl) chipEl.remove(); chipEl = null; return; }
      ensureUI();
      if (!chipEl || !chipEl.isConnected) { chipEl = uiDoc.createElement('div'); root.appendChild(chipEl); }
      chipEl.className = 'chip' + (enabled ? '' : ' off');
      chipEl.textContent = enabled ? 'म' : 'EN';
      chipEl.title = enabled ? 'Marathi typing ON – click or Alt+M for English' : 'English – click or Alt+M for Marathi';
      let left, top;
      if (docsFrame) {
        left = uiDoc.documentElement.clientWidth - 34; top = uiDoc.documentElement.clientHeight - 34;
      } else {
        const r = el.getBoundingClientRect();
        if (!r.width || !r.height) { chipEl.remove(); chipEl = null; return; }
        left = r.right - 26;
        top = r.height < 40 ? r.top + (r.height - 22) / 2 : r.bottom - 26;
      }
      chipEl.style.left = Math.max(0, left) + 'px'; chipEl.style.top = Math.max(0, top) + 'px';
    });
  }
  window.addEventListener('focusin', updateChip, true);
  window.addEventListener('focusout', () => setTimeout(updateChip, 0), true);
  window.addEventListener('scroll', updateChip, { capture: true, passive: true });
  window.addEventListener('resize', updateChip, { passive: true });

  let toastTimer = 0;
  function toast(msg) {
    ensureUI();
    let t = root.querySelector('.toast');
    if (!t) { t = uiDoc.createElement('div'); t.className = 'toast'; root.appendChild(t); }
    t.textContent = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => t.remove(), 1400);
  }
})();
