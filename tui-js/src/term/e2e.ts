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
const SHELL_SESSION = sessionName(SLICE, "shell"); // slis-shell/checkout
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
  await sh(["tmux", "kill-session", "-t", SHELL_SESSION]);
  const shell = process.env["SHELL"] ?? "bash";
  await sh(["tmux", "new-session", "-d", "-s", SESSION, "-x", "200", "-y", "50", shell]);
  await sh(["tmux", "new-session", "-d", "-s", SHELL_SESSION, "-x", "200", "-y", "50", shell]);

  const vt = new PersistentTerminal({ cols: APP_COLS, rows: APP_ROWS });
  let tearingDown = false;

  const appCmd = process.env["SLIS_E2E_BIN"]
    ? [process.env["SLIS_E2E_BIN"]!]
    : ["bun", "run", "src/index.tsx"];
  const app: any = Bun.spawn(appCmd, {
    cwd: `${import.meta.dir}/../..`,
    env: { ...process.env, TERM: "xterm-256color", SLIS_FAKE: "1" },
    terminal: {
      cols: APP_COLS,
      rows: APP_ROWS,
      data(_t: unknown, data: Uint8Array) {
        if (!tearingDown) vt.feed(data);
      },
    },
  } as any);
  const pty = app.terminal;

  await sleep(2800); // boot: renderer + fake data + first paint
  const boot = vt.getText();
  const sawBrowser = boot.includes(SLICE) && boot.toLowerCase().includes("slices");

  // A compact installed-binary smoke mode isolates the global quit path from
  // tmux/session setup: browser → create overlay → ctrl+c → process exit.
  if (process.env["SLIS_E2E_QUIT_ONLY"] === "1") {
    pty.write("c");
    await sleep(300);
    const sawCreate = vt.getText().includes("Create slice");
    pty.write("\x03");
    const quit = await Promise.race([
      app.exited.then(() => true),
      sleep(1200).then(() => false),
    ]);
    tearingDown = true;
    try { app.kill(); } catch {}
    vt.destroy();
    await sh(["tmux", "kill-session", "-t", SESSION]);
    await sh(["tmux", "kill-session", "-t", SHELL_SESSION]);
    console.log(`installed_browser=${sawBrowser} create_overlay=${sawCreate} ctrl_c_quit=${quit}`);
    process.exit(sawBrowser && sawCreate && quit ? 0 : 1);
  }

  // Enter the cockpit and move through stack rows with real arrow sequences.
  // The breadcrumb is fixed chrome: selection/render changes must never make
  // the terminal viewport scroll it away.
  pty.write("\r");
  await sleep(700);
  const cockpitOpened = vt.getText().includes(`slis › ${SLICE}`);
  pty.write("\x1b[B\x1b[B\x1b[B");
  await sleep(700);
  const breadcrumbSurvivesArrows = vt.getText().includes(`slis › ${SLICE}`);
  pty.write("\x1b");
  await sleep(500);

  // Open the focused slice's terminal tab (attach only — no agent launch).
  pty.write("a");
  await sleep(1200);
  const inTab = vt.getText();
  const sawTabBar = inTab.includes("term") && inTab.includes(SLICE);
  const browserHidden =
    !inTab.includes("FILTERS") && !inTab.includes("CHANGES") && !inTab.includes("SLICES");

  // Type into the EMBEDDED shell; the marker must round-trip and be painted.
  const marker = "SLIS_EMBED_" + Date.now();
  pty.write(`printf '${marker}\\n'\r`);
  await sleep(1200);
  const sawMarker = vt.getText().includes(marker);

  // ctrl+c belongs to the embedded terminal while it has focus. Prove the app
  // survives it and the PTY still accepts a subsequent command.
  pty.write("\x03");
  await sleep(300);
  const afterInterruptMarker = "SLIS_AFTER_CTRL_C_" + Date.now();
  pty.write(`printf '${afterInterruptMarker}\\n'\r`);
  await sleep(700);
  const ctrlCReachedTerminal = vt.getText().includes(afterInterruptMarker);

  // ctrl+q must return to the browser (and NOT reach the shell).
  pty.write("\x11");
  await sleep(900);
  const backText = vt.getText();
  const backToBrowser = backText.includes("SLICES") && !backText.includes("ctrl+q back");

  // A separate shell tab must coexist with the agent tab and attach to its own
  // tmux namespace rather than replacing or typing into the agent session.
  pty.write("t");
  await sleep(900);
  const shellTabText = vt.getText();
  const sawShellTab = shellTabText.includes(`${SLICE} · shell`);
  const bothSessionsAlive =
    (await sh(["tmux", "has-session", "-t", SESSION])).code === 0 &&
    (await sh(["tmux", "has-session", "-t", SHELL_SESSION])).code === 0;
  pty.write("\x11");
  await sleep(700);

  // Ctrl+C must also quit while an overlay owns React keyboard focus. Open the
  // create window first; this catches regressions where ctrl+c is normalized to
  // plain `c` or swallowed by the modal instead of reaching the global guard.
  pty.write("c");
  await sleep(300);
  const createOverlayOpen = vt.getText().includes("Create slice");
  pty.write("\x03");
  const ctrlCQuitsBrowser = await Promise.race([
    app.exited.then(() => true),
    sleep(1200).then(() => false),
  ]);
  const sessionAlive = (await sh(["tmux", "has-session", "-t", SESSION])).code === 0;

  tearingDown = true;
  try {
    app.kill();
  } catch {
    // best-effort teardown
  }
  const lastPaint = vt.getText();
  await sleep(150);
  vt.destroy();
  await sh(["tmux", "kill-session", "-t", SESSION]);
  await sh(["tmux", "kill-session", "-t", SHELL_SESSION]);

  const R: Record<string, boolean> = {
    browser_paints_slice_list: sawBrowser,
    enter_opens_cockpit: cockpitOpened,
    breadcrumb_survives_arrow_navigation: breadcrumbSurvivesArrows,
    key_a_opens_terminal_tab: sawTabBar,
    terminal_surface_hides_browser: browserHidden,
    keystrokes_reach_embedded_shell: sawMarker,
    ctrl_c_reaches_embedded_terminal: ctrlCReachedTerminal,
    ctrl_q_returns_to_browser: backToBrowser,
    shell_tab_is_separate: sawShellTab && bothSessionsAlive,
    c_opens_create_overlay: createOverlayOpen,
    ctrl_c_quits_from_overlay: ctrlCQuitsBrowser,
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
