import type { RpcClient } from "./types";
import { SlisRpcClient } from "./client";
import { FakeRpcClient } from "./fake";

export * from "./types";
export { RpcError } from "./client";

/**
 * Build the RPC client. SLIS_FAKE=1 selects the in-process fixtures; otherwise
 * we spawn the real `slis rpc` sidecar (binary from $SLIS_BIN, default "slis").
 */
export function createRpcClient(): RpcClient {
  if (process.env["SLIS_FAKE"] === "1") return new FakeRpcClient();
  return new SlisRpcClient();
}

export function usingFake(): boolean {
  return process.env["SLIS_FAKE"] === "1";
}
