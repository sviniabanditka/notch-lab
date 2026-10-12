// From Search's AgentPage.swift (MIT): the body of an async function.
let el = null;
if (ref) {
  const held = window.__notchRefs && window.__notchRefs.get(ref);
  el = held && held.deref();
  if (!el || !el.isConnected) return { gone: 'ref ' + ref + ' is gone — take a new browser.snapshot' };
} else if (selector) {
  if (selector.startsWith('text=')) {
    const want = selector.slice(5).trim().toLowerCase();
    el = Array.from(document.querySelectorAll('button, a, [role=button], input[type=submit], input[type=button], label, summary'))
      .find(e => ((e.innerText || e.value || '').trim().toLowerCase()) === want) || null;
  } else {
    try { el = document.querySelector(selector); } catch (e) { return { gone: 'not a selector: ' + selector }; }
  }
  if (!el) return { gone: 'nothing matches ' + selector };
} else {
  return { gone: 'give a ref or a selector' };
}
el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' });
const box = el.getBoundingClientRect();
let x = box.left + box.width / 2, y = box.top + box.height / 2;
let w = el.ownerDocument.defaultView;
while (w && w !== window && w.frameElement) {
  const f = w.frameElement.getBoundingClientRect();
  x += f.left + (w.frameElement.clientLeft || 0);
  y += f.top + (w.frameElement.clientTop || 0);
  w = w.parent;
}
const out = { x, y, width: box.width, height: box.height, tag: el.tagName.toLowerCase() };
if (el.ownerDocument === document) {
  const top = document.elementFromPoint(x, y);
  if (top && top !== el && !el.contains(top) && !top.contains(el)) {
    out.covered = '<' + top.tagName.toLowerCase() + (top.id ? '#' + top.id : '') + '>';
  }
}
return out;
