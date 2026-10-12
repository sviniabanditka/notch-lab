// From Search's AgentPage.swift (MIT): the body of an async function.
const el = document.activeElement;
if (!el) return false;
if (el.isContentEditable) { document.execCommand('selectAll'); document.execCommand('delete'); return true; }
if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
  const proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value');
  if (setter && setter.set) setter.set.call(el, ''); else el.value = '';
  el.dispatchEvent(new Event('input', { bubbles: true }));
  return true;
}
return false;
