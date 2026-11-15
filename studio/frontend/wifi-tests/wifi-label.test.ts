import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { buildWiFiItem, wifiLabel, type DocLike, type ElLike } from "../src/wifi-label.ts";

// A fake document whose elements throw when something that parses markup is used, and record what text was set.
function fakeDoc() {
  const made: { tag: string; el: ElLike }[] = [];
  const doc: DocLike = {
    createElement(tag: string) {
      const el = new Proxy(
        { className: "", textContent: null as string | null, dataset: {} as Record<string, string | undefined>, children: [] as ElLike[], append(...c: ElLike[]) { (el as any).children.push(...c); } },
        {
          set(target, prop, value) {
            if (prop === "innerHTML" || prop === "outerHTML") throw new Error(`${String(prop)} was set`);
            (target as any)[prop] = value;
            return true;
          },
          get(target, prop) {
            if (prop === "insertAdjacentHTML") throw new Error("insertAdjacentHTML was used");
            return (target as any)[prop];
          },
        },
      ) as unknown as ElLike;
      made.push({ tag, el });
      return el;
    },
  };
  return { doc, made };
}

describe("the Wi-Fi device item", () => {
  it("shows the name and the address as the text it was given", () => {
    const hostile = { name: '<img src=x onerror="window.__wifiXss=1">', host: "10.0.0.7", port: 48686, version: "<b>1</b>" };
    const { doc, made } = fakeDoc();
    const li = buildWiFiItem(doc, hostile, "10.0.0.7:48686") as any;
    assert.equal(li.dataset.key, "10.0.0.7:48686");
    assert.equal(li.children.length, 2);
    const [name, host] = li.children;
    assert.equal(name.textContent, hostile.name); // text, not markup: the fake would throw on innerHTML
    assert.equal(host.textContent, "10.0.0.7:48686 · v<b>1</b>");
    assert.deepEqual(made.map((m) => m.tag), ["li", "strong", "span"]);
    assert.equal(host.className, "wifi-host");
  });
  it("leaves out the version when there is none", () => {
    assert.deepEqual(wifiLabel({ name: "Pixel 8", host: "192.168.1.5", port: 48686, version: "" }), { name: "Pixel 8", detail: "192.168.1.5:48686" });
    assert.equal(wifiLabel({ name: "x", host: "h", port: 1, version: "0.27.0" }).detail, "h:1 · v0.27.0");
  });
});
