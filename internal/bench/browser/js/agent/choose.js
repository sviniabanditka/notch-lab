// From Search's AgentPage.swift (MIT): the body of an async function.
const held = ref && window.__notchRefs && window.__notchRefs.get(ref);
const el = held ? held.deref() : (selector ? document.querySelector(selector) : null);
if (!el || el.tagName !== 'SELECT') return 'not a select';
const want = values.map(v => String(v).trim().toLowerCase());
let hit = 0;
for (const o of el.options) {
  const on = want.includes(o.value.toLowerCase()) || want.includes(o.text.trim().toLowerCase());
  if (on) hit++;
  if (el.multiple || on) o.selected = on;
}
if (!hit) return 'no option matches ' + values.join(', ');
el.dispatchEvent(new Event('input', { bubbles: true }));
el.dispatchEvent(new Event('change', { bubbles: true }));
return 'ok';
