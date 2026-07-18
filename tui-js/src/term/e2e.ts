// End-to-end proof for the embedded terminal tabs: run the REAL app (src/index.tsx)
// inside a Bun PTY with the fake sidecar, drive it, and read what it actually
// paints (via ghostty's parser) — no human at a terminal.
//
// Proves: browser paints → `a` opens a terminal tab attached to a live tmux
// session → keystrokes reach the embedded shell → ctrl+q returns to the browser
// → quitting slis leaves the tmux session ALIVE (we only ever detach the client).
//
// Run: bun run src/term/e2e.ts   (requires tmux; skips cleanly if absent)

import { PersistentTerminal } from "ghostty-opentui";
import { sessionName } from "./tmux";

const APP_COLS = 160;
const APP_ROWS = 44;
const SLICE = "checkout"; // first fake slice; focused at index 0 under filter "All"
const SESSION = sessionName(SLICE); // slis/checkout
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

  // Pre-create the slice's tmux session so the app's ensureSession is a no-op and
  // attaches to a known shell (no dependency on real worktrees or on claude).
  await sh(["tmux", "kill-session", "-t", SESSION]);
  const shell = process.env["SHELL"] ?? "bash";
  await sh(["tmux", "new-session", "-d", "-s", SESSION, "-x", "200", "-y", "50", shell]);

  const vt = new PersistentTerminal({ cols: APP_COLS, rows: APP_ROWS });

  const app: any = Bun.spawn(["bun", "run", "src/index.tsx"], {
    cwd: `${import.meta.dir}/../..`,
    env: { ...process.env, TERM: "xterm-256color", SLIS_FAKE: "1" },
    terminal: {
      cols: APP_COLS,
      rows: APP_ROWS,
      data(_t: unknown, data: Uint8Array) {
        vt.feed(data);
      },
    },
  } as any);
  const pty = app.terminal;

  await sleep(2800); // boot: renderer + fake data + first paint
  const boot = vt.getText();
  const sawBrowser = boot.includes(SLICE) && boot.toLowerCase().includes("slices");

  // Open the focused slice's terminal tab (attach only — no agent launch).
  pty.write("a");
  await sleep(1200);
  const inTab = vt.getText();
  const sawTabBar = inTab.includes("term") && inTab.includes(SLICE);

  // Type into the EMBEDDED shell; the marker must round-trip and be painted.
  const marker = "SLIS_EMBED_" + Date.now();
  pty.write(`printf '${marker}\\n'\r`);
  await sleep(1200);
  const sawMarker = vt.getText().includes(marker);

  // ctrl+q must return to the browser (and NOT reach the shell).
  pty.write("\x11");
  await sleep(900);
  const backText = vt.getText();
  const backToBrowser = backText.includes("refresh") || backText.includes("filter");

  // Quit slis; the tmux session must survive (we only detach the client).
  pty.write("q");
  await sleep(1200);
  const sessionAlive = (await sh(["tmux", "has-session", "-t", SESSION])).code === 0;

  try {
    app.kill();
  } catch {
    // best-effort teardown
  }
  const lastPaint = vt.getText();
  vt.destroy();
  await sh(["tmux", "kill-session", "-t", SESSION]);

  const R: Record<string, boolean> = {
    browser_paints_slice_list: sawBrowser,
    key_a_opens_terminal_tab: sawTabBar,
    keystrokes_reach_embedded_shell: sawMarker,
    ctrl_q_returns_to_browser: backToBrowser,
    tmux_session_survives_app_quit: sessionAlive,
  };
  console.log("\n===== term-tabs E2E (drove real src/index.tsx in a PTY) =====");
  for (const [k, v] of Object.entries(R)) console.log(`  ${(v ? "PASS" : "FAIL").padEnd(5)} ${k}`);
  console.log("============================================================");
  const ok = Object.values(R).every(Boolean);
  if (!ok) {
    console.log("\n--- last paint (first 16 lines) ---\n" + lastPaint.split("\n").slice(0, 16).join("\n"));
  }
  process.exit(ok ? 0 : 1);
}

main().catch((e) => {
  console.error("[term e2e] ERROR", e);
  process.exit(1);
});
