// From Search's AgentPage.swift (MIT): the body of an async function.
if (selector) { try { if (document.querySelector(selector)) return true; } catch (e) { return false; } }
if (text) return (document.body ? document.body.innerText : '').includes(text);
return false;
