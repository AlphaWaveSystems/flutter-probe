import * as http from "node:http";

/** A fake OpenAI-compatible provider on 127.0.0.1. It records "METHOD /path" for every request and answers a chat with reply. */
export interface FakeProvider {
  url: string;
  paths: () => string[];
  chats: () => number;
  close: () => Promise<void>;
}

export async function fakeProvider(reply = "pong"): Promise<FakeProvider> {
  const seen: string[] = [];
  const server = http.createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      seen.push(`${req.method} ${req.url}`);
      res.setHeader("Content-Type", "application/json");
      if (req.method === "POST" && (req.url ?? "").endsWith("/chat/completions")) {
        res.end(JSON.stringify({ choices: [{ message: { content: reply } }] }));
        return;
      }
      res.end(JSON.stringify({ data: [{ id: "fake-model" }] }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as { port: number }).port;
  return {
    url: `http://127.0.0.1:${port}`,
    paths: () => [...seen],
    chats: () => seen.filter((p) => p.startsWith("POST") && p.endsWith("/chat/completions")).length,
    close: () => new Promise((resolve) => server.close(() => resolve())),
  };
}
