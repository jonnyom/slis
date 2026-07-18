// Visual proof for the agent picker: run the REAL app (src/index.tsx) inside a
// Bun PTY with the fake sidecar (whose hello now returns TWO agents), press `C`
// to launch an agent, and read what it actually paints.
//
// Proves: `C` with >1 configured agent opens the picker overlay listing the
// agents → quick-pick "2" selects codex and opens a session tab titled with the
// agent name → the picker is gone.
//
// Run: bun run src/term/agentpicker.e2e.ts   (requires tmux; skips cleanly if absent)

import { PersistentTerminal } from "ghostty-opentui";
import { sessionName } from "./tmux";

const APP_COLS = 160;
const APP_ROWS = 44;
const SLICE = "checkout"; // first fake slice; focused at index 0 under filter "All"
const SESSION = sessionName(SLICE);
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function sh(cmd: string[]) {
  const p = Bun.spawn(cmd, { stdout: "pipe", stderr: "pipe", stdin: "ignore" });
  const out = await new Response(p.stdout).text();
  const code = await p.exited;
  return { code, out: out.trim() };
}

async function main() {
  if (Bun.which("tmux") === null) {
    console.log("SKIP: tmux not on PATH");
    process.exit(0);
  }

  // Pre-create the slice's tmux session so the launch attaches to a known shell
  // (no dependency on real worktrees or on claude/codex being installed).
  await sh(["tmux", "kill-session", "-t", SESSION]);
  const shell = process.env["SHELL"] ?? "bash";
  await sh(["tmux", "new-session", "-d", "-s", SESSION, "-x", "200", "-y", "50", shell]);

  const vt = new PersistentTerminal({ cols: APP_COLS, rows: APP_ROWS });
  let tearingDown = false;
  const app: any = Bun.spawn(["bun", "run", "src/index.tsx"], {
    cwd: `${import.meta.dir}/../..`,
    env: { ...process.env, TERM: "xterm-256color", SLIS_FAKE: "1" },
    terminal: {
      cols: APP_COLS,
      rows: APP_ROWS,
      data(_t: unknown, data: Uint8Array) {
        if (!tearingDown) vt.feed(data); // ignore buffered flush after kill/destroy
      },
    },
  } as any);
  const pty = app.terminal;

  await sleep(2800); // boot: renderer + fake data + first paint

  // Press C: launch agent. With two fake agents, the picker overlay must appear.
  pty.write("C");
  await sleep(900);
  const pickerText = vt.getText();
  const sawPicker =
    pickerText.includes("Launch which agent?") &&
    pickerText.includes("claude") &&
    pickerText.includes("codex");

  // Quick-pick "2" → codex; a session tab opens titled with the agent name.
  pty.write("2");
  await sleep(1400);
  const tabText = vt.getText();
  const pickerGone = !tabText.includes("Launch which agent?");
  const sawAgentTab = tabText.includes(SLICE) && tabText.includes("codex");

  const lastPaint = vt.getText();
  tearingDown = true;
  try {
    app.kill();
  } catch {
    // best-effort teardown
  }
  await sleep(150); // let in-flight PTY data drain into the ignored callback
  vt.destroy();
  await sh(["tmux", "kill-session", "-t", SESSION]);

  const R: Record<string, boolean> = {
    C_opens_agent_picker_with_both_agents: sawPicker,
    quick_pick_dismisses_picker: pickerGone,
    session_tab_titled_with_agent_name: sawAgentTab,
  };
  console.log("\n===== agent-picker E2E (drove real src/index.tsx in a PTY) =====");
  for (const [k, v] of Object.entries(R)) console.log(`  ${(v ? "PASS" : "FAIL").padEnd(5)} ${k}`);
  console.log("===============================================================");
  const ok = Object.values(R).every(Boolean);
  if (!ok) {
    console.log("\n--- last paint (first 20 lines) ---\n" + lastPaint.split("\n").slice(0, 20).join("\n"));
  }
  process.exit(ok ? 0 : 1);
}

main().catch((e) => {
  console.error("[agent-picker e2e] ERROR", e);
  process.exit(1);
});
