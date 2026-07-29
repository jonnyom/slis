// Guards the read-coalescing contract of SlisRpcClient against a fake sidecar:
// identical reads issued while one is in flight must become ONE request, so a
// slow fan-out (conflicts) can never queue burst after burst behind the 30s tick.

import { afterEach, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, writeFileSync, chmodSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { SlisRpcClient } from "./client";

// A sidecar stand-in: logs every request method it receives, answers after a
// delay long enough for a second caller to arrive while the first is pending.
const sidecarScript = (log: string) => `#!/usr/bin/env bun
const log = ${JSON.stringify(log)};
for await (const line of console) {
  const text = line.trim();
  if (!text) continue;
  const req = JSON.parse(text);
  await Bun.write(log, (await Bun.file(log).text().catch(() => "")) + req.method + "\\n");
  await Bun.sleep(120);
  process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: req.id, result: { overlaps: [], incomplete: [] } }) + "\\n");
}
`;

let client: SlisRpcClient | null = null;
let logPath = "";

function startClient(): SlisRpcClient {
  const dir = mkdtempSync(join(tmpdir(), "slis-rpc-coalesce-"));
  const script = join(dir, "fake-sidecar");
  logPath = join(dir, "requests.log");
  writeFileSync(script, sidecarScript(logPath));
  chmodSync(script, 0o755);
  writeFileSync(logPath, "");
  client = new SlisRpcClient({ bin: script });
  return client;
}

function requestCount(method: string): number {
  if (!existsSync(logPath)) return 0;
  return readFileSync(logPath, "utf8")
    .split("\n")
    .filter((line) => line.trim() === method).length;
}

afterEach(() => {
  client?.close();
  client = null;
});

test("identical concurrent reads become one sidecar request", async () => {
  const c = startClient();

  const [a, b, d] = await Promise.all([c.conflicts(), c.conflicts(), c.conflicts()]);

  expect(requestCount("conflicts")).toBe(1);
  expect(a).toEqual(b);
  expect(b).toEqual(d);
});

test("reads with different params are not coalesced together", async () => {
  const c = startClient();

  await Promise.all([c.show("alpha"), c.show("beta"), c.show("alpha")]);

  expect(requestCount("show")).toBe(2);
});

test("a settled read does not serve the next call from cache", async () => {
  const c = startClient();

  await c.conflicts();
  await c.conflicts();

  expect(requestCount("conflicts")).toBe(2);
});

test("a client-side timeout withdraws the request from the sidecar", async () => {
  const dir = mkdtempSync(join(tmpdir(), "slis-rpc-cancel-"));
  const script = join(dir, "fake-sidecar");
  logPath = join(dir, "requests.log");
  writeFileSync(script, sidecarScript(logPath));
  chmodSync(script, 0o755);
  writeFileSync(logPath, "");
  // The fake answers after 120ms; give up after 20ms so the timeout path runs.
  client = new SlisRpcClient({ bin: script, requestTimeoutMs: 20 });

  await expect(client.conflicts()).rejects.toThrow(/rpc timeout: conflicts/);

  // The fake reads the next line only after answering the first, so poll for the
  // cancel rather than guessing a sleep.
  const deadline = Date.now() + 3_000;
  while (requestCount("cancel") === 0 && Date.now() < deadline) await Bun.sleep(20);
  expect(requestCount("cancel")).toBe(1);
});
