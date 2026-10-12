// From Search's AgentPage.swift (MIT): the body of an async function.
const held = ref && window.__notchRefs && window.__notchRefs.get(ref);
const el = held && held.deref();
if (ref) {
  if (!el) return { gone: 'ref ' + ref + ' is gone — take a new browser.snapshot' };
  el.scrollIntoView({ block: 'center', behavior: 'instant' });
} else {
  // The thing under the middle of the view that scrolls, or the page.
  let s = document.elementFromPoint(innerWidth / 2, innerHeight / 2);
  while (s && s !== document.body && !(s.scrollHeight > s.clientHeight && /auto|scroll/.test(getComputedStyle(s).overflowY))) s = s.parentElement;
  (s && s !== document.body ? s : document.scrollingElement || document.documentElement).scrollBy({ left: dx, top: dy, behavior: 'instant' });
}
return { x: scrollX, y: scrollY, height: document.documentElement.scrollHeight };
