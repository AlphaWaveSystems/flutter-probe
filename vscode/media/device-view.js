/* PLACEHOLDER: replaced by `npm run copy-device-view` once the shared front-end
 * bundle (studio/frontend/dist-device-view/device-view.js) is built. It only
 * implements the mountDeviceView API surface so the extension can be exercised;
 * it draws nothing and never opens a connection. */
(function () {
  window.mountDeviceView = function (container, opts) {
    var note = document.createElement('div');
    note.style.cssText = 'padding:16px;font-family:sans-serif';
    note.textContent = 'Device view bundle not installed (placeholder). Build studio/frontend and run npm run copy-device-view.';
    container.appendChild(note);
    if (opts && opts.onStatus) { opts.onStatus({ t: 'status', state: 'error', msg: 'placeholder bundle' }); }
    return {
      destroy: function () { note.remove(); },
      setConfig: function () {},
      send: function () {},
    };
  };
})();
