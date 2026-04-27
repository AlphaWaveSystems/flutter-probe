import { defineConfig } from "vite";

// Monaco's worker scripts are loaded via the `MonacoEnvironment.getWorker`
// hook (defined inline in main.ts where needed). For our MVP we only use
// the basic editor — no JSON/HTML/CSS/TS workers — so the default Vite
// config is sufficient.
//
// Output goes to `frontend/dist` which Wails embeds via `//go:embed
// all:frontend/dist`.
export default defineConfig({
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2020",
    sourcemap: true,
  },
});
