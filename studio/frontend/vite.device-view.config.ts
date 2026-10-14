import { defineConfig } from "vite";

// Single-file bundles (ESM for Studio and the demo page, IIFE with the global FlutterProbeDeviceView for the VS Code webview) of the device view component, for the Studio
// frontend, a plain browser page and a VS Code webview. No Wails or VS Code
// code is included. Output: dist-device-view/device-view.js
export default defineConfig({
  publicDir: false,
  build: {
    outDir: "dist-device-view",
    emptyOutDir: true,
    target: "es2020",
    sourcemap: false,
    minify: true,
    lib: {
      entry: "src/device-view/index.ts",
      formats: ["es", "iife"],
      name: "FlutterProbeDeviceView",
      fileName: (format) => (format === "es" ? "device-view.js" : "device-view.iife.js"),
    },
  },
});
