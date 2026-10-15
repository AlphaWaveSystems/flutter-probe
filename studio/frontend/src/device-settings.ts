// Device settings panel: OS-level settings (dark mode, font size, app language, orientation, deep link) of the
// connected emulator or simulator. The Go side (devsettings) decides what is offered and checks every value; this file
// only draws the controls and reports the outcome. It never names a device: changes go to the connected one.

import { ApplyDeviceSetting, GetDeviceSettings } from "../wailsjs/go/main/App";
import { toast } from "./toast";

interface Capability {
  setting: string;
  label: string;
  choices?: string[];
  needsApp?: boolean;
  note?: string;
}

/** The control for one capability: a select for a fixed list, a text field otherwise. Exported for tests. */
export function controlKind(c: Pick<Capability, "choices">): "select" | "text" {
  return c.choices && c.choices.length > 0 ? "select" : "text";
}

const el = <T extends HTMLElement>(tag: string, cls?: string, text?: string): T => {
  const e = document.createElement(tag) as T;
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
};

let panel: HTMLElement | null = null;

async function refresh(): Promise<void> {
  if (!panel) return;
  panel.textContent = "";
  let info;
  try {
    info = await GetDeviceSettings();
  } catch (err) {
    panel.appendChild(el("div", "ds-note", `Could not read the device: ${err}`));
    return;
  }
  const head = el("div", "ds-head", info.deviceName ? `${info.deviceName} (${info.platform})` : "No device");
  panel.appendChild(head);
  if (!info.available) {
    panel.appendChild(el("div", "ds-note", info.reason || "Not available."));
    return;
  }
  const appRow = el("div", "ds-row");
  appRow.appendChild(el("label", "ds-label", "App id"));
  const appInput = el<HTMLInputElement>("input", "ds-input");
  appInput.id = "ds-app";
  appInput.value = info.app ?? "";
  appInput.placeholder = "com.example.app (for language and links)";
  appRow.appendChild(appInput);
  panel.appendChild(appRow);

  for (const c of info.capabilities ?? []) {
    const row = el("div", "ds-row");
    row.appendChild(el("label", "ds-label", c.label));
    let control: HTMLSelectElement | HTMLInputElement;
    if (controlKind(c) === "select") {
      const sel = el<HTMLSelectElement>("select", "ds-input");
      for (const v of c.choices ?? []) sel.appendChild(new Option(v, v));
      control = sel;
    } else {
      const inp = el<HTMLInputElement>("input", "ds-input");
      inp.placeholder = c.setting === "locale" ? "de, pt-BR" : "myapp://item/1";
      control = inp;
    }
    control.id = `ds-${c.setting}`;
    row.appendChild(control);
    const btn = el<HTMLButtonElement>("button", "btn-mini", "Apply");
    btn.id = `ds-apply-${c.setting}`;
    btn.addEventListener("click", async () => {
      btn.disabled = true;
      try {
        await ApplyDeviceSetting(c.setting, control.value, c.needsApp || c.setting === "open_url" ? appInput.value : "");
        toast(`${c.label}: ${control.value}`, "success", 2500);
      } catch (err) {
        toast(`${c.label}: ${err}`, "error", 6000);
      } finally {
        btn.disabled = false;
      }
    });
    row.appendChild(btn);
    panel.appendChild(row);
    if (c.note) panel.appendChild(el("div", "ds-note", c.note));
  }
  panel.appendChild(
    el("div", "ds-note", "Settings stay on the device until you change them back."),
  );
}

export function initDeviceSettings(): void {
  panel = document.getElementById("device-settings");
  const btn = document.getElementById("btn-device-settings");
  if (!panel || !btn) return;
  btn.addEventListener("click", () => {
    const open = panel!.hidden;
    panel!.hidden = !open;
    btn.setAttribute("aria-expanded", String(open));
    if (open) void refresh();
  });
}
