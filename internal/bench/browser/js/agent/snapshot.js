// From Search's AgentPage.swift (MIT): the body of an async function.
// Arguments: max (characters), prefix (ref prefix). Refs hold their
// elements strongly until the next snapshot: a WeakRef holds this world's
// wrapper, which nothing else holds, and goes at the next collection.
const SECRET = /^(authorization|proxy-authorization|cookie|set-cookie|auth|sid|key|code|sig)$|token|secret|passw|pwd|passcode|otp|csrf|xsrf|api[-_]?key|apikey|credential|session[-_]?id|signature|private[-_]?key|security[-_]?token/i;
const maskURL = (u) => {
  try {
    const x = new URL(u, location.href);
    if (x.username) x.username = '•••';
    if (x.password) x.password = '•••';
    for (const k of [...x.searchParams.keys()]) if (SECRET.test(k)) x.searchParams.set(k, '•••');
    return x.href;
  } catch (e) { return String(u); }
};
const MAX = max || 40000;
const frames = [];
const refs = new Map();
window.__notchRefs = refs;
let n = 0, size = 0, cut = false;
const out = [];
const push = (depth, s) => {
  if (cut) return;
  const line = '  '.repeat(depth) + '- ' + s;
  size += line.length + 1;
  if (size > MAX) { cut = true; return; }
  out.push(line);
};
const q = s => JSON.stringify(s);
const clean = s => String(s || '').replace(/\s+/g, ' ').trim();
const short = (s, most = 80) => { s = clean(s); return s.length > most ? s.slice(0, most - 1) + '…' : s; };
const INTERACTIVE = new Set(['link', 'button', 'textbox', 'searchbox', 'checkbox', 'radio', 'combobox', 'listbox', 'slider', 'switch', 'tab', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'option', 'spinbutton', 'treeitem', 'clickable']);
// Roles whose name is their text: what is inside them is said already.
const WORDED = new Set(['link', 'button', 'heading', 'tab', 'menuitem', 'option', 'checkbox', 'radio', 'switch', 'textbox', 'searchbox', 'combobox', 'listbox', 'img', 'clickable', 'columnheader']);
// Said only to be nothing in particular.
const PLAIN = new Set(['none', 'presentation', 'generic']);
const SKIP = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'HEAD', 'META', 'LINK', 'svg', 'SVG', 'PATH', 'BR', 'HR']);
function role(el, style) {
  const r = el.getAttribute('role');
  if (r) { const first = r.split(/\s+/)[0]; return PLAIN.has(first) ? null : first; }
  switch (el.tagName) {
    case 'A': return el.hasAttribute('href') ? 'link' : null;
    case 'BUTTON': case 'SUMMARY': return 'button';
    case 'SELECT': return el.multiple || el.size > 1 ? 'listbox' : 'combobox';
    case 'TEXTAREA': return 'textbox';
    case 'INPUT': {
      const t = (el.type || 'text').toLowerCase();
      if (t === 'hidden') return null;
      if (['button', 'submit', 'reset', 'image', 'file', 'color'].includes(t)) return 'button';
      if (t === 'checkbox') return 'checkbox';
      if (t === 'radio') return 'radio';
      if (t === 'range') return 'slider';
      if (t === 'search') return 'searchbox';
      if (t === 'number') return 'spinbutton';
      return 'textbox';
    }
    case 'H1': case 'H2': case 'H3': case 'H4': case 'H5': case 'H6': return 'heading';
    case 'IMG': return clean(el.getAttribute('alt')) ? 'img' : null;
    case 'NAV': return 'navigation';
    case 'MAIN': return 'main';
    case 'ASIDE': return 'complementary';
    case 'FORM': return 'form';
    case 'DIALOG': return 'dialog';
    case 'HEADER': return el.closest('article, aside, main, nav, section') ? null : 'banner';
    case 'FOOTER': return el.closest('article, aside, main, nav, section') ? null : 'contentinfo';
    case 'UL': case 'OL': return 'list';
    case 'LI': return 'listitem';
    case 'TABLE': return 'table';
    case 'TR': return 'row';
    case 'TH': return 'columnheader';
    case 'TD': return 'cell';
    case 'P': return 'paragraph';
    case 'IFRAME': case 'FRAME': return 'iframe';
  }
  if (el.isContentEditable && (!el.parentElement || !el.parentElement.isContentEditable)) return 'textbox';
  // A div a framework listens to: a pointer over it, and not only
  // because what it sits in has one.
  if (style.cursor === 'pointer' && el.parentElement && getComputedStyle(el.parentElement).cursor !== 'pointer') return 'clickable';
  return null;
}
function name(el, r) {
  let s = el.getAttribute('aria-label');
  if (!s) {
    const ids = el.getAttribute('aria-labelledby');
    const root = el.getRootNode();
    if (ids) s = ids.split(/\s+/).map(i => { const x = (root.getElementById ? root : el.ownerDocument).getElementById(i); return x ? x.innerText || x.textContent : ''; }).join(' ');
  }
  if (!s && el.labels && el.labels.length) s = Array.from(el.labels).map(l => l.innerText).join(' ');
  if (!s && el.tagName === 'IMG') s = el.alt;
  if (!s && el.tagName === 'INPUT' && ['button', 'submit', 'reset'].includes(el.type)) s = el.value;
  if (!s && WORDED.has(r) && !['textbox', 'searchbox', 'combobox', 'listbox'].includes(r)) s = el.innerText;
  if (!s) s = el.getAttribute('title') || el.getAttribute('placeholder') || '';
  return short(s, 80);
}
function kids(el) {
  if (el.shadowRoot) return Array.from(el.shadowRoot.childNodes);
  if (el.tagName === 'SLOT') return el.assignedNodes({ flatten: true });
  return Array.from(el.childNodes);
}
function walk(el, depth, words) {
  for (const c of kids(el)) {
    if (cut) return;
    if (c.nodeType === 3) {
      if (!words) continue;
      const t = short(c.data, 200);
      if (t) push(depth, 'text ' + q(t));
      continue;
    }
    if (c.nodeType !== 1 || SKIP.has(c.tagName)) continue;
    if (c.getAttribute('aria-hidden') === 'true') continue;
    const style = getComputedStyle(c);
    if (style.display === 'none') continue;
    // Hidden itself, though what is in it may be shown.
    if (style.visibility === 'hidden') { walk(c, depth, false); continue; }
    const r = role(c, style);
    if (!r) { walk(c, depth, words); continue; }
    if (r === 'iframe') {
      let inner = null;
      try { inner = c.contentDocument; } catch (e) {}
      push(depth, 'iframe ' + q(short(c.src ? maskURL(c.src) : '', 120)) + (inner ? '' : ' (another site)'));
      if (!inner && c.src && !cut) {
        // Its element kept, for where its content is on the page. The
        // raw address is for the app to find the frame by; it is taken
        // out of the answer before the agent gets it (Agents.snapshot).
        const key = 'frame' + frames.length;
        refs.set(key, { deref: () => c });
        frames.push({ src: c.src, line: out.length - 1, depth, key });
      }
      if (inner && inner.body) walk(inner.body, depth + 1, true);
      continue;
    }
    const nm = name(c, r);
    if (['list', 'listitem', 'row', 'table', 'form'].includes(r) && !nm && !c.children.length && !clean(c.textContent)) continue;
    let line = r + (nm ? ' ' + q(nm) : '');
    if (INTERACTIVE.has(r)) {
      const box = c.getBoundingClientRect();
      let target = c;
      if (box.width === 0 && box.height === 0 && style.position !== 'fixed') {
        // A checkbox drawn by its label, the box itself out of sight:
        // the label is what a hand would click.
        const label = (r === 'checkbox' || r === 'radio') && c.closest('label');
        if (!label) { walk(c, depth, words); continue; }
        target = label;
      }
      const id = (prefix || 'e') + (++n);
      refs.set(id, { deref: () => target });
      line += ' [ref=' + id + ']';
    }
    if (r === 'heading') line += ' [level=' + (c.getAttribute('aria-level') || (/^H[1-6]$/.test(c.tagName) ? c.tagName[1] : 2)) + ']';
    const checked = c.getAttribute('aria-checked') || ((c.type === 'checkbox' || c.type === 'radio') ? String(c.checked) : null);
    if (checked === 'true') line += ' checked';
    if (c.disabled || c.getAttribute('aria-disabled') === 'true') line += ' disabled';
    if (c.getAttribute('aria-expanded')) line += ' expanded=' + c.getAttribute('aria-expanded');
    if (c.getAttribute('aria-selected') === 'true') line += ' selected';
    if (c.required) line += ' required';
    if (r === 'textbox' || r === 'searchbox' || r === 'spinbutton') {
      const v = c.isContentEditable ? c.innerText : c.value;
      line += ' value=' + (c.type === 'password' ? (v ? '"•••"' : '""') : q(short(v || '', 120)));
    }
    if (r === 'combobox' || r === 'listbox') {
      if (c.tagName === 'SELECT') {
        const chosen = Array.from(c.selectedOptions || []).map(o => clean(o.text));
        const all = Array.from(c.options || []).slice(0, 12).map(o => clean(o.text));
        line += ' value=' + q(chosen.join(', ')) + ' options=' + q(all.join(' | ') + (c.options.length > 12 ? ' | …' : ''));
      } else if (c.value !== undefined) {
        line += ' value=' + q(short(c.value || '', 120));
      }
    }
    if (r === 'link') {
      const href = c.getAttribute('href') || '';
      if (href && !href.startsWith('javascript:')) line += ' url=' + q(short(maskURL(c.href), 120));
    }
    push(depth, line);
    if (!['textbox', 'searchbox', 'combobox', 'listbox', 'img', 'spinbutton', 'slider'].includes(r)) {
      walk(c, depth + 1, !WORDED.has(r) || !nm);
    }
  }
}
push(0, 'document ' + q(document.title) + ' url=' + q(maskURL(location.href)));
// A page nested past what the stack holds is cut where it got to.
try { if (document.body) walk(document.body, 1, true); } catch (e) { cut = true; }
return { outline: out.join('\n'), cut, refs: n, frames };
