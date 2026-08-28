import { expect, test } from "bun:test";
import { LiveMirrorManager, liveSyncCommand, type LiveMirrorProcess } from "./mirror";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function fakeProcess(stderr = "") {
  const exit = deferred<number>();
  let ended = 0;
  const process: LiveMirrorProcess = {
    stdin: {
      end() {
        ended++;
        exit.resolve(0);
      },
    },
    stderr: new Response(stderr).body!,
    exited: exit.promise,
    kill() {},
  };
  return { process, exit, ended: () => ended };
}

test("live mirror command uses the current slis binary", () => {
  expect(liveSyncCommand("/tmp/slis")).toEqual(["/tmp/slis", "live-sync"]);
});

test("live mirror manager starts once and stops through stdin", async () => {
  const child = fakeProcess();
  const commands: string[][] = [];
  const warnings: string[] = [];
  const manager = new LiveMirrorManager(
    (warning) => warnings.push(warning),
    (command) => {
      commands.push(command);
      return child.process;
    },
    "/tmp/slis",
  );

  manager.start();
  manager.start();
  expect(commands).toEqual([["/tmp/slis", "live-sync"]]);

  await manager.stop();

  expect(child.ended()).toBe(1);
  expect(warnings).toEqual([]);
  expect(manager.running).toBe(false);
});

test("live mirror manager reports unexpected process failure", async () => {
  const child = fakeProcess("unknown primary changes");
  const warnings: string[] = [];
  const manager = new LiveMirrorManager(
    (warning) => warnings.push(warning),
    () => child.process,
    "/tmp/slis",
  );

  manager.start();
  child.exit.resolve(1);
  await child.process.exited;
  await Bun.sleep(0);

  expect(warnings).toEqual(["unknown primary changes"]);
  expect(manager.running).toBe(false);
});
