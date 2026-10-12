(() => {
  if (window.__searchProbe) return;
  window.__searchProbe = true;
  const q = [];
  let size = 0, timer = 0;
  const flush = () => {
    timer = 0;
    if (!q.length) return;
    size = 0;
    try { webkit.messageHandlers.%HANDLER%.postMessage('[' + q.splice(0).join(',') + ']'); } catch (e) {}
  };
  document.addEventListener('%EVENT%', e => {
    if (typeof e.detail !== 'string' || e.detail.length > 200000) return;
    q.push(e.detail);
    size += e.detail.length;
    // At most a few megabytes on their way at once: the oldest go.
    while (q.length > 500 || size > 4000000) size -= q.shift().length;
    if (!timer) timer = setTimeout(flush, 100);
  }, true);
  addEventListener('pagehide', flush, true);
  // Said once from every frame, for the app to know it is there:
  // a frame from another site is read through what it says here.
  try { webkit.messageHandlers.%HANDLER%.postMessage('[]'); } catch (e) {}
})();
