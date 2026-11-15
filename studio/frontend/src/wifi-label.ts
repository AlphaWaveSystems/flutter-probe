// The list item for a device found on the local network. What a device announces (its name, its version) is text from any machine on
// the network: Go bounds and cleans it (entryToDevice), and this builds the item with textContent only, so whatever is left is shown as
// plain text and never parsed as markup. The element and the document are minimal interfaces so a test can hand in a recording fake.

export type WiFiDevice = { name: string; host: string; port: number; version: string };

export interface ElLike {
  className: string;
  textContent: string | null;
  dataset: Record<string, string | undefined>;
  append(...children: ElLike[]): void;
}

export interface DocLike {
  createElement(tag: string): ElLike;
}

/** The two lines of the item: the name, and where it is (and its version, when it announced one). */
export function wifiLabel(dev: WiFiDevice): { name: string; detail: string } {
  return { name: dev.name, detail: `${dev.host}:${dev.port}${dev.version ? ` · v${dev.version}` : ""}` };
}

/** The list item: the name in bold and the address under it, both set as text. The caller adds what it needs (a click, a saved mark). */
export function buildWiFiItem(doc: DocLike, dev: WiFiDevice, key: string): ElLike {
  const label = wifiLabel(dev);
  const li = doc.createElement("li");
  li.dataset.key = key;
  const nameEl = doc.createElement("strong");
  nameEl.textContent = label.name;
  const hostEl = doc.createElement("span");
  hostEl.className = "wifi-host";
  hostEl.textContent = label.detail;
  li.append(nameEl, hostEl);
  return li;
}
