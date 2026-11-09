import { expect, test } from "bun:test";
import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

test("keys typed while a session tab is still attaching reach the terminal", async () => {
  const directory = mkdtempSync(join(tmpdir(), "slis-session-"));
  const fakeSlis = join(directory, "slis");
  writeFileSync(
    fakeSlis,
    `#!/bin/sh
case "$2" in
  ensure|ensure-tab) sleep 0.3; echo '{"id":"feature","tabs":[]}' ;;
  attach) exec cat ;;
esac
`,
  );
  chmodSync(fakeSlis, 0o755);

  const child = Bun.spawn([process.execPath, "-e", `
    import { TermSession } from "./src/term/session";
    const session = new TermSession("feature");
    let output = "";
    const attaching = session.attach(80, 24, (bytes) => { output += new TextDecoder().decode(bytes); }, {
      slice: "feature", kind: "shell", tabID: "shell-1", tabTitle: "shell", members: [],
      active: false, wsRoot: "", sessionOpts: {}, launchAgent: false, agent: "", harness: "",
    });
    session.write("typed-before-attach");
    await attaching;
    const deadline = Date.now() + 2000;
    while (!output.includes("typed-before-attach") && Date.now() < deadline) await Bun.sleep(20);
    session.detach();
    process.stdout.write(output.includes("typed-before-attach") ? "received" : "dropped");
  `], {
    cwd: new URL("../..", import.meta.url).pathname,
    env: { ...process.env, SLIS_BIN: fakeSlis },
    stdout: "pipe",
    stderr: "pipe",
  });
  const [stdout, stderr] = await Promise.all([
    new Response(child.stdout).text(),
    new Response(child.stderr).text(),
    child.exited,
  ]);
  expect(stderr).toBe("");
  expect(stdout).toBe("received");
});
