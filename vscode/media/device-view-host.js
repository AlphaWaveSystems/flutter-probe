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
        // Only the element fields cross to the extension, which builds the step.
        if (!node) { return; }
        vscode.postMessage({
          type: 'select',
          node: { key: node.key, text: node.text, label: node.label, type: node.type },
        });
      },
      onStatus: function (s) {
        var state = s && s.state ? String(s.state) : 'error';
        vscode.postMessage({ type: 'status', state: state });
        if (state === 'taken') { vscode.postMessage({ type: 'taken' }); }
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
