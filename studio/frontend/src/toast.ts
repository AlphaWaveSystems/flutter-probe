// Lightweight non-blocking notification helper. Toasts auto-dismiss; the
// container is rendered in index.html and stays anchored to the bottom right.

export type ToastKind = "info" | "success" | "error";

export function toast(message: string, kind: ToastKind = "info", durationMs = 3500): void {
  const container = document.getElementById("toast-container");
  if (!container) return;
  const el = document.createElement("div");
  el.classList.add("toast", kind);
  el.textContent = message;
  container.appendChild(el);
  window.setTimeout(() => {
    el.style.opacity = "0";
    el.style.transform = "translateY(4px)";
    el.style.transition = "all 0.18s";
    window.setTimeout(() => el.remove(), 200);
  }, durationMs);
}
