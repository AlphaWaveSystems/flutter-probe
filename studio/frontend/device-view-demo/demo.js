import { mountDeviceView } from "../dist-device-view/device-view.js";

const params = new URLSearchParams(location.search);
const url = params.get("url") || "ws://127.0.0.1:8765/device/v1";
const token = params.get("token") || "demo";
const logEl = document.getElementById("log");

function log(line) {
  logEl.textContent = (line + "\n" + logEl.textContent).slice(0, 4000);
}

const view = mountDeviceView(document.getElementById("view"), {
  url,
  token,
  onSelect: (n) => log("select " + JSON.stringify(n)),
  onStatus: (s) => log("status " + s.state + (s.msg ? " (" + s.msg + ")" : "")),
});

document.getElementById("slow").addEventListener("click", () => view.setConfig({ fps: 2 }));
document.getElementById("fast").addEventListener("click", () => view.setConfig({ fps: 15, tree: true }));
window.addEventListener("pagehide", () => view.destroy());
