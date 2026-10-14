// Copies the shared device view front-end bundle (built from studio/frontend)
// into vscode/media/. If the bundle has not been built, the tracked placeholder
// stays in place and a warning is printed.
const fs = require('fs');
const path = require('path');

const src = path.resolve(__dirname, '..', '..', 'studio', 'frontend', 'dist-device-view');
const dst = path.resolve(__dirname, '..', 'media');
const bundle = path.join(src, 'device-view.js');

if (!fs.existsSync(bundle)) {
  console.warn(`[copy-device-view] ${bundle} not found; keeping the placeholder in media/.`);
  process.exit(0);
}
fs.mkdirSync(dst, { recursive: true });
for (const name of fs.readdirSync(src)) {
  fs.copyFileSync(path.join(src, name), path.join(dst, name));
  console.log(`[copy-device-view] ${name}`);
}
