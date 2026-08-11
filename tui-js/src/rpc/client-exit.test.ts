import { afterEach, expect, test } from "bun:test";
import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { SlisRpcClient } from "./client";

let client: SlisRpcClient | null = null;

afterEach(() => {
  client?.close();
  client = null;
});

test("reports process exit when a descendant keeps stderr open", async () => {
  const directory = mkdtempSync(join(tmpdir(), "slis-rpc-exit-"));
  const script = join(directory, "fake-sidecar");
  writeFileSync(script, "#!/bin/sh\nsleep 2 &\necho diagnostic >&2\nexit 1\n");
  chmodSync(script, 0o755);

  client = new SlisRpcClient({ bin: script });

  const error = await new Promise<Error>((resolve, reject) => {
    const timeout = setTimeout(
      () => reject(new Error("connection failure was not reported")),
      2_000,
    );
    client!.onConnectionChange((connected, connectionError) => {
      if (!connected && connectionError) {
        clearTimeout(timeout);
        resolve(connectionError);
      }
    });
  });

  expect(error.message).toBe("sidecar exited (1)");
});

test("reports stderr when it closes with the process", async () => {
  const directory = mkdtempSync(join(tmpdir(), "slis-rpc-exit-"));
  const script = join(directory, "fake-sidecar");
  writeFileSync(script, "#!/bin/sh\necho diagnostic >&2\nexit 1\n");
  chmodSync(script, 0o755);

  client = new SlisRpcClient({ bin: script });

  const error = await new Promise<Error>((resolve) => {
    client!.onConnectionChange((connected, connectionError) => {
      if (!connected && connectionError) resolve(connectionError);
    });
  });

  expect(error.message).toBe("diagnostic");
});
