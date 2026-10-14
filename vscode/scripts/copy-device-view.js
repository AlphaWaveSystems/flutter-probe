// Copies the shared device view front-end bundle (built from studio/frontend)
// into vscode/media/. If the bundle has not been built, the tracked placeholder
// stays in place and a warning is printed.
const fs = require('fs');
const path = require('path');

const src = path.resolve(__dirname, '..', '..', 'studio', 'frontend', 'dist-device-view');
const dst = path.resolve(__dirname, '..', 'media');
// The webview loads a classic script, so it gets the IIFE build (global FlutterProbeDeviceView), saved as media/device-view.js.
const bundle = path.join(src, 'device-view.iife.js');

if (!fs.existsSync(bundle)) {
  console.warn(`[copy-device-view] ${bundle} not found; keeping the placeholder in media/.`);
  process.exit(0);
}
fs.mkdirSync(dst, { recursive: true });
fs.copyFileSync(bundle, path.join(dst, 'device-view.js'));
console.log('[copy-device-view] device-view.iife.js -> media/device-view.js');
