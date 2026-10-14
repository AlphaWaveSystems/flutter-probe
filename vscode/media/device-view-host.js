/* Glue between the VS Code extension host and the shared device view bundle.
 * The bundle must expose mountDeviceView on window (directly or under
 * window.FlutterProbeDeviceView). The hub token arrives by postMessage and is
 * kept only in memory. */
(function () {
  var vscode = acquireVsCodeApi();
  var root = document.getElementById('root');
  var view = null;
  var visible = true;

  function mountFn() {
    if (typeof window.mountDeviceView === 'function') { return window.mountDeviceView; }
    var ns = window.FlutterProbeDeviceView;
    return ns && typeof ns.mountDeviceView === 'function' ? ns.mountDeviceView : null;
  }

  function selectorFor(node) {
    if (!node) { return ''; }
    if (node.key) { return '#' + node.key; }
    if (node.text) { return '"' + node.text + '"'; }
    return node.label || node.type || '';
  }

  function mount(msg) {
    var mountDeviceView = mountFn();
    if (!mountDeviceView) {
      root.textContent = 'Device view bundle is missing mountDeviceView.';
      vscode.postMessage({ type: 'status', state: 'error' });
      return;
    }
    if (view) { view.destroy(); view = null; root.textContent = ''; }
    view = mountDeviceView(root, {
      url: msg.url,
      token: msg.token,
      onSelect: function (node) {
        var s = selectorFor(node);
        if (s) { vscode.postMessage({ type: 'select', selector: s }); }
      },
      onStatus: function (s) {
        vscode.postMessage({ type: 'status', state: s && s.state ? String(s.state) : 'error' });
      },
    });
    if (msg.config) { view.setConfig(msg.config); }
    applyVisibility();
  }

  function applyVisibility() {
    if (view) { view.send({ t: visible ? 'resume' : 'pause' }); }
  }

  window.addEventListener('message', function (event) {
    var msg = event.data;
    if (!msg || typeof msg !== 'object') { return; }
    if (msg.type === 'init') { mount(msg); }
    else if (msg.type === 'visibility') { visible = !!msg.visible; applyVisibility(); }
    else if (msg.type === 'hub-exit') {
      if (view) { view.destroy(); view = null; }
      root.textContent = 'The device view hub has stopped.';
    }
  });

  vscode.postMessage({ type: 'ready' });
})();
