const SECRET = new RegExp('%PATTERN%', 'i');
const MASK = '%MASK%';
const maskParams = text => text.split('&').map(p => {
  const name = p.split('=')[0];
  let plain = name;
  try { plain = decodeURIComponent(name); } catch (e) {}
  return p.includes('=') && SECRET.test(plain) ? name + '=' + MASK : p;
}).join('&');
const maskURL = u => {
  try {
    const x = new URL(u, location.href);
    const q = maskParams(x.search.slice(1));
    const h = x.hash.slice(1);
    return x.protocol + '//' + x.host + x.pathname + (q ? '?' + q : '') + (h ? '#' + (h.includes('=') ? maskParams(h) : h) : '');
  } catch (e) { return String(u); }
};
const scrub = text => String(text).replace(/\b[a-z][a-z0-9+.-]*:\/\/[^\s"'<>()]+/gi, u => maskURL(u));