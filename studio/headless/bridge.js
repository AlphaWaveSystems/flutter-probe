// Studio headless bridge: stands in for the Wails runtime in a normal browser, so the real
// frontend can be driven by Playwright without a native window. Served by Studio itself when it
// is started with PROBE_STUDIO_HEADLESS=1; it is never part of the desktop app.
(function () {
  const TOKEN = "__TOKEN__";

  async function call(method, args) {
    const resp = await fetch("/__headless/call", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Headless-Token": TOKEN },
      body: JSON.stringify({ method, args }),
    });
    const out = await resp.json();
    if (out.error) throw out.error;
    return out.result;
  }

  window.go = {
    main: {
      App: new Proxy({}, { get: (_t, method) => (...args) => call(String(method), args) }),
    },
  };

  const listeners = {}; // event name -> [{cb, left}]
  const source = new EventSource("/__headless/events?token=" + encodeURIComponent(TOKEN));
  source.onmessage = (e) => {
    const { name, data } = JSON.parse(e.data);
    const list = listeners[name];
    if (!list) return;
    for (const l of [...list]) {
      l.cb(...(data || []));
      if (l.left > 0 && --l.left === 0) list.splice(list.indexOf(l), 1);
    }
  };
  window.__headlessEventsReady = new Promise((resolve) => { source.onopen = () => resolve(true); });

  const noop = () => {};
  window.runtime = new Proxy(
    {
      EventsOnMultiple(name, cb, max) {
        const l = { cb, left: max > 0 ? max : 0 };
        (listeners[name] = listeners[name] || []).push(l);
        return () => {
          const list = listeners[name] || [];
          const i = list.indexOf(l);
          if (i >= 0) list.splice(i, 1);
        };
      },
      EventsOff(name) { delete listeners[name]; },
      EventsOffAll() { for (const k of Object.keys(listeners)) delete listeners[k]; },
      EventsEmit: noop,
      LogPrint: noop, LogTrace: noop, LogDebug: noop, LogInfo: noop, LogWarning: noop, LogError: noop, LogFatal: noop,
    },
    {
      get(target, prop) {
        if (prop in target) return target[prop];
        return () => Promise.resolve(); // window, clipboard, browser helpers: nothing to do without a window
      },
    }
  );
})();
