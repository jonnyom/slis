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

test("delivers frontend focus notifications", async () => {
  const directory = mkdtempSync(join(tmpdir(), "slis-rpc-focus-"));
  const script = join(directory, "fake-sidecar");
  writeFileSync(script, "#!/bin/sh\nsleep 0.05\nprintf '%s\\n' '{\"jsonrpc\":\"2.0\",\"method\":\"focusSession\",\"params\":{\"id\":\"focus-1\",\"group_id\":\"feature\",\"tab_id\":\"agent\",\"time_ns\":1}}'\nsleep 1\n");
  chmodSync(script, 0o755);
  client = new SlisRpcClient({ bin: script });
  const request = await new Promise<{ id: string; group_id: string; tab_id: string; time_ns: number }>((resolve) => {
    client!.onFocusRequest(resolve);
  });
  expect(request).toEqual({ id: "focus-1", group_id: "feature", tab_id: "agent", time_ns: 1 });
});
