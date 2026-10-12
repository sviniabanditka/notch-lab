(() => {
  const EV = '%EVENT%';
  // What is used here, taken before the page can change it.
  const stringify = JSON.stringify, parse = JSON.parse, Custom = CustomEvent, Proxy_ = Proxy, Reflect_ = Reflect;
  const dispatch = EventTarget.prototype.dispatchEvent, listen = EventTarget.prototype.addEventListener;
  const clock = performance.now.bind(performance);
  const keysOf = Object.keys, own = Object.getOwnPropertyDescriptor;
  const doc = Math.random().toString(36).slice(2, 8);
  let seq = 0;
  const say = o => { try { dispatch.call(document, new Custom(EV, { detail: stringify(o) })); } catch (e) {} };
  const cap = (s, n) => (s.length > n ? s.slice(0, n) + '…' : s);

  // A value as a console shows it: a few levels, a few keys each, no
  // getters run, and never more than it has room for — a whole app's
  // state logged by mistake costs nothing.
  const nodeName = v => v.nodeType === 1
    ? '<' + v.tagName.toLowerCase() + (v.id ? '#' + v.id : '') + (typeof v.className === 'string' && v.className.trim() ? '.' + v.className.trim().split(/\s+/).join('.') : '') + '>'
    : '#' + (v.nodeName || 'node').toLowerCase();
  const show = (v, depth, room) => {
    if (v === null) return 'null';
    if (v === undefined) return 'undefined';
    const t = typeof v;
    if (t === 'string') return depth ? stringify(cap(v, 200)) : v;
    if (t === 'number' || t === 'boolean' || t === 'bigint') return String(v);
    if (t === 'function') return 'ƒ ' + (v.name || 'anonymous') + '()';
    if (t === 'symbol') return 'Symbol(' + (v.description || '') + ')';
    try {
      if (v instanceof Error) return v.name + ': ' + v.message + (!depth && v.stack ? '\n' + v.stack : '');
      if (typeof Node !== 'undefined' && v instanceof Node) return nodeName(v);
      if (v === window) return 'Window';
    } catch (e) {}
    const list = Array.isArray(v);
    if (depth >= 3) return list ? '[…]' : '{…}';
    let keys;
    try { keys = keysOf(v); } catch (e) { return '{…}'; }
    const parts = [];
    for (const k of keys.slice(0, 50)) {
      if (room.left <= 0) { parts.push('…'); break; }
      let said;
      try {
        const d = own(v, k);
        said = SECRET.test(k) ? MASK
          : !d || !('value' in d) ? '(getter)'
          : show(d.value, depth + 1, room);
      } catch (e) { said = '?'; }
      const part = list ? said : k + ': ' + said;
      room.left -= part.length;
      parts.push(part);
    }
    if (keys.length > 50) parts.push('… ' + (keys.length - 50) + ' more');
    return list ? '[' + parts.join(', ') + ']' : '{' + parts.join(', ') + '}';
  };
  const ser = v => cap(show(v, 0, { left: 2000 }), 2000);

  // What must not reach an agent: credentials, by the names they go by.
  %SECRETS%
  const heads = h => {
    const o = {};
    try { h.forEach((v, k) => { o[k] = SECRET.test(k) ? MASK : cap(String(v), 1000); }); } catch (e) {}
    return o;
  };
  const plainHeads = o => {
    const out = {};
    for (const k of keysOf(o)) out[k] = SECRET.test(k) ? MASK : cap(String(o[k]), 1000);
    return out;
  };
  const parseHeads = raw => {
    const o = {};
    (raw || '').trim().split(/[\r\n]+/).forEach(line => {
      const i = line.indexOf(':');
      if (i > 0) o[line.slice(0, i).trim().toLowerCase()] = line.slice(i + 1).trim();
    });
    return plainHeads(o);
  };
  const where = u => cap(maskURL(u), 4000);
  const maskJSON = (v, depth) => {
    if (v === null || typeof v !== 'object' || depth > 20) return v;
    if (Array.isArray(v)) return v.map(x => maskJSON(x, depth + 1));
    const o = {};
    // Under a secret's name, whatever is there — a list of tokens, an object of credentials.
    for (const k of keysOf(v)) o[k] = SECRET.test(k) ? MASK : maskJSON(v[k], depth + 1);
    return o;
  };
  const BODY = 65536;
  const masked = (text, type) => {
    try {
      if (/json/i.test(type) || /^\s*[\[{]/.test(text)) return stringify(maskJSON(parse(text), 0));
    } catch (e) {}
    try {
      if (/urlencoded/i.test(type) || /^[\w.%\[\]-]+=[^&]*(&[\w.%\[\]-]+=[^&]*)*$/.test(text)) {
        const p = new URLSearchParams(text);
        for (const k of [...p.keys()]) if (SECRET.test(k)) p.set(k, MASK);
        return p.toString();
      }
    } catch (e) {}
    return text;
  };
  const bodyOf = (b, type) => {
    try {
      if (b == null) return undefined;
      if (typeof b === 'string') return cap(masked(b, type || ''), BODY);
      if (b instanceof URLSearchParams) return cap(masked(b.toString(), 'urlencoded'), BODY);
      if (typeof FormData !== 'undefined' && b instanceof FormData) {
        const parts = [];
        b.forEach((v, k) => parts.push(k + '=' + (SECRET.test(k) ? MASK : typeof v === 'string' ? v : '[file ' + (v.name || '') + ']')));
        return cap(parts.join('&'), BODY);
      }
      return '[' + ((b.constructor && b.constructor.name) || 'body') + ']';
    } catch (e) { return undefined; }
  };
  // Text worth keeping; not a stream that may never end.
  const TEXTY = /json|text|xml|javascript|graphql|x-www-form-urlencoded/i;
  const STREAM = /event-stream|ndjson|stream|multipart/i;
  const readable = type => TEXTY.test(type) && !STREAM.test(type);

  // The console.
  for (const level of ['log', 'info', 'warn', 'error', 'debug', 'trace', 'assert']) {
    const was = console[level];
    if (typeof was !== 'function') continue;
    console[level] = function (...args) {
      try {
        let said = args;
        if (level === 'assert') { if (args[0]) return was.apply(this, args); said = ['Assertion failed:'].concat(args.slice(1)); }
        const o = { kind: 'console', level, text: scrub(said.map(ser).join(' ')) };
        if (level !== 'log' && level !== 'info' && level !== 'debug') {
          o.stack = scrub(String(new Error().stack || '').split('\n').slice(1, 8).join('\n'));
        }
        say(o);
      } catch (e) {}
      return was.apply(this, args);
    };
  }

  // The page's own errors, and what failed to load.
  const workerURLs = new Set();
  listen.call(window, 'error', e => {
    const el = e.target;
    if (el && el !== window && el.tagName) {
      const src = el.currentSrc || el.src || (el.href && el.href.baseVal) || el.href || '';
      say({ kind: 'net', phase: 'resource', id: doc + 'r' + (++seq), via: el.tagName.toLowerCase(), url: where(String(src)), failed: 'failed to load' });
      return;
    }
    // An error in a worker is heard from its Worker, below.
    if (e.filename && workerURLs.has(e.filename)) return;
    say({ kind: 'error', level: 'error', text: scrub(e.message || 'error'), source: e.filename ? maskURL(e.filename) : undefined, line: e.lineno, column: e.colno, stack: e.error && e.error.stack ? scrub(e.error.stack) : undefined });
  }, true);
  listen.call(window, 'unhandledrejection', e => {
    say({ kind: 'error', level: 'error', text: scrub('Unhandled rejection: ' + ser(e.reason)), stack: e.reason && e.reason.stack ? scrub(e.reason.stack) : undefined });
  });
  listen.call(document, 'securitypolicyviolation', e => {
    say({ kind: 'error', level: 'error', text: 'Content Security Policy: ' + e.violatedDirective + ' blocked ' + (e.blockedURI ? maskURL(e.blockedURI) : 'inline') });
  });

  // fetch: the same promise the page gets, and a copy of the answer
  // read on the side, a little of it, for a few seconds at most.
  const readBody = async (res, id) => {
    try {
      const type = res.headers.get('content-type') || '';
      if (!readable(type) || !res.body) return;
      const reader = res.clone().body.getReader();
      const dec = new TextDecoder();
      const until = clock() + 5000;
      let text = '', size = 0, cut = false;
      while (size < BODY) {
        const next = await Promise.race([reader.read(), new Promise(r => setTimeout(() => r(null), Math.max(0, until - clock())))]);
        if (!next) { cut = true; break; }
        if (next.done) break;
        size += next.value.length;
        text += dec.decode(next.value, { stream: true });
      }
      if (size >= BODY) cut = true;
      if (cut) reader.cancel().catch(() => {});
      say({ kind: 'net', phase: 'body', id, body: cap(masked(text, type), BODY), cut });
    } catch (e) {}
  };
  const oldFetch = window.fetch;
  if (typeof oldFetch === 'function') {
    const fetch = function (input, init) {
      const id = doc + 'f' + (++seq);
      const start = clock();
      try {
        let method = 'GET', url = '', h = {}, type = '';
        if (typeof Request !== 'undefined' && input instanceof Request) { method = input.method; url = input.url; h = heads(input.headers); type = input.headers.get('content-type') || ''; }
        else url = String(input);
        let body;
        if (init) {
          if (init.method) method = String(init.method).toUpperCase();
          if (init.headers) { const all = new Headers(init.headers); h = heads(all); type = all.get('content-type') || type; }
          body = bodyOf(init.body, type);
        }
        say({ kind: 'net', phase: 'start', id, via: 'fetch', method, url: where(url), requestHeaders: h, requestBody: body });
      } catch (e) {}
      let p;
      try { p = oldFetch.apply(this, arguments); } catch (e) {
        say({ kind: 'net', phase: 'end', id, failed: String((e && e.message) || e), ms: 0 });
        throw e;
      }
      p.then(res => {
        say({ kind: 'net', phase: 'end', id, status: res.status, statusText: res.statusText, type: res.headers.get('content-type') || '', responseHeaders: heads(res.headers), ms: Math.round(clock() - start), redirected: res.redirected || undefined });
        readBody(res, id);
      }, err => {
        say({ kind: 'net', phase: 'end', id, failed: String((err && err.message) || err), ms: Math.round(clock() - start) });
      });
      return p;
    };
    window.fetch = fetch;
  }

  // XMLHttpRequest, heard once per send even when it is used again.
  const X = window.XMLHttpRequest && XMLHttpRequest.prototype;
  if (X) {
    const started = new WeakMap();
    const open = X.open, send = X.send, setHeader = X.setRequestHeader;
    X.open = function (method, url) {
      try { started.set(this, { method: String(method).toUpperCase(), url: where(url), h: {} }); } catch (e) {}
      return open.apply(this, arguments);
    };
    X.setRequestHeader = function (k, v) {
      try { const s = started.get(this); if (s) s.h[String(k).toLowerCase()] = String(v); } catch (e) {}
      return setHeader.apply(this, arguments);
    };
    X.send = function (b) {
      const s = started.get(this);
      if (s) {
        const id = doc + 'x' + (++seq);
        const start = clock();
        say({ kind: 'net', phase: 'start', id, via: 'xhr', method: s.method, url: s.url, requestHeaders: plainHeads(s.h), requestBody: bodyOf(b, s.h['content-type']) });
        listen.call(this, 'loadend', () => {
          try {
            const type = this.getResponseHeader('content-type') || '';
            const o = { kind: 'net', phase: 'end', id, status: this.status, type, ms: Math.round(clock() - start), responseHeaders: parseHeads(this.getAllResponseHeaders()) };
            if (this.status === 0) o.failed = 'network error';
            if ((this.responseType === '' || this.responseType === 'text') && readable(type)) o.body = cap(masked(this.responseText || '', type), BODY);
            else if (this.responseType === 'json' && this.response != null) o.body = cap(stringify(maskJSON(this.response, 0)), BODY);
            say(o);
          } catch (e) {}
        }, { once: true });
      }
      return send.apply(this, arguments);
    };
  }

  // WebSocket: when it opens and closes, and how much went each way.
  // A Proxy, not a function in its place: `extends WebSocket`,
  // `instanceof`, its name and its constants all stay the page's.
  const OldSocket = window.WebSocket;
  if (typeof OldSocket === 'function') {
    const sent = new WeakMap();
    const wsSend = OldSocket.prototype.send;
    OldSocket.prototype.send = function () { sent.set(this, (sent.get(this) || 0) + 1); return wsSend.apply(this, arguments); };
    window.WebSocket = new Proxy_(OldSocket, {
      construct(target, args, made) {
        const ws = Reflect_.construct(target, args, made);
        try {
          const id = doc + 'w' + (++seq);
          let got = 0;
          say({ kind: 'net', phase: 'start', id, via: 'websocket', method: 'GET', url: where(ws.url) });
          listen.call(ws, 'message', () => { got++; });
          listen.call(ws, 'close', e => say({ kind: 'net', phase: 'end', id, status: e.code, received: got, sent: sent.get(ws) || 0 }));
        } catch (e) {}
        return ws;
      },
    });
  }

  // Workers: what goes wrong in one reaches its Worker here. With
  // workers on, a worker of the page's own site is started inside a
  // wrapper that sends its console back as a message of ours, taken
  // off before the page's own listeners hear it.
  const WRAP = %WORKERS%;
  const TAG = EV + 'w';
  const prelude = (url, module) => `
    (() => {
      const TAG = ${stringify(TAG)}, BASE = ${stringify(url)};
      const SECRET = new RegExp(${stringify(SECRET.source)}, 'i'), MASK = ${stringify(MASK)};
      const post = self.postMessage.bind(self);
      // Bounded, and a secret's value masked by its name, as the page's own console.
      const ser = v => {
        try {
          if (typeof v === 'string') return v;
          if (v instanceof Error) return v.name + ': ' + v.message;
          const seen = new WeakSet();
          let n = 0;
          const s = JSON.stringify(v, (k, x) => {
            if (++n > 200) return undefined;
            if (k && SECRET.test(k)) return MASK;
            if (x && typeof x === 'object') { if (seen.has(x)) return '[Circular]'; seen.add(x); }
            return typeof x === 'bigint' ? String(x) : x;
          });
          return s === undefined ? String(v) : s;
        } catch (e) { return '[unreadable]'; }
      };
      for (const level of ['log', 'info', 'warn', 'error', 'debug']) {
        const was = console[level];
        console[level] = function (...a) { try { post({ [TAG]: { level, text: a.map(ser).join(' ').slice(0, 2000) } }); } catch (e) {} return was.apply(this, a); };
      }
      self.addEventListener('unhandledrejection', e => { try { post({ [TAG]: { level: 'error', text: 'Unhandled rejection: ' + ser(e.reason) } }); } catch (x) {} });
      const fix = u => { try { return new URL(u, BASE).href; } catch (e) { return u; } };
      if (self.importScripts) { const was = self.importScripts; self.importScripts = (...u) => was.apply(self, u.map(fix)); }
      const f = self.fetch; self.fetch = (i, o) => f.call(self, typeof i === 'string' || i instanceof URL ? fix(String(i)) : i, o);
      const x = XMLHttpRequest.prototype.open; XMLHttpRequest.prototype.open = function (m, u, ...r) { return x.call(this, m, fix(u), ...r); };
    })();
    ${module ? 'await import(' + stringify(url) + ');' : 'importScripts(' + stringify(url) + ');'}
  `;
  const watchWorker = (w, url) => {
    workerURLs.add(url);
    try {
      listen.call(w, 'error', e => say({ kind: 'error', level: 'error', text: scrub('Worker: ' + (e.message || 'error')), source: maskURL(e.filename || url), line: e.lineno }));
      listen.call(w, 'message', e => {
        const said = e.data && typeof e.data === 'object' ? e.data[TAG] : null;
        if (!said) return;
        e.stopImmediatePropagation();
        say({ kind: 'console', level: String(said.level), text: scrub(said.text), worker: maskURL(url) });
      });
    } catch (e) {}
    return w;
  };
  const OldWorker = window.Worker;
  if (typeof OldWorker === 'function') {
    window.Worker = new Proxy_(OldWorker, {
      construct(target, args, made) {
        const url = String(args[0]), whole = new URL(url, location.href);
        if (WRAP && whole.origin === location.origin && !url.startsWith('blob:') && !url.startsWith('data:')) {
          try {
            const module = args[1] && args[1].type === 'module';
            const blob = URL.createObjectURL(new Blob([prelude(whole.href, module)], { type: 'text/javascript' }));
            const w = Reflect_.construct(target, [blob].concat(args.slice(1)), made);
            setTimeout(() => URL.revokeObjectURL(blob), 10000);
            return watchWorker(w, whole.href);
          } catch (e) {}
        }
        return watchWorker(Reflect_.construct(target, args, made), whole.href);
      },
    });
  }
  const OldShared = window.SharedWorker;
  if (typeof OldShared === 'function') {
    window.SharedWorker = new Proxy_(OldShared, {
      construct(target, args, made) {
        const w = Reflect_.construct(target, args, made);
        try { listen.call(w, 'error', e => say({ kind: 'error', level: 'error', text: scrub('Shared worker: ' + (e.message || 'error')), source: where(String(args[0])) })); } catch (e) {}
        return w;
      },
    });
  }

  // Everything else it loaded: pictures, scripts, styles, fonts.
  try {
    new PerformanceObserver(list => {
      for (const e of list.getEntries()) {
        if (e.initiatorType === 'fetch' || e.initiatorType === 'xmlhttprequest') continue;
        say({ kind: 'net', phase: 'resource', id: doc + 'r' + (++seq), via: e.initiatorType || 'other', url: where(e.name), ms: Math.round(e.duration), size: e.transferSize || undefined, status: e.responseStatus || undefined });
      }
    }).observe({ type: 'resource', buffered: true });
  } catch (e) {}
})();