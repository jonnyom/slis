import { PersistentTerminal } from "ghostty-opentui";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { TerminalQueryResponder } from "./responses";

const APP_COLS = 160;
const APP_ROWS = 44;
const SLICE = "checkout";
const AGENT_TAB = "agent-claude";
const sleep = (milliseconds: number) => new Promise((resolveSleep) => setTimeout(resolveSleep, milliseconds));
const projectRoot = resolve(import.meta.dir, "../../..");
const tuiRoot = resolve(import.meta.dir, "../..");

async function run(command: string[], env: Record<string, string | undefined> = process.env) {
  const child = Bun.spawn(command, { stdout: "pipe", stderr: "pipe", stdin: "ignore", env });
  const [stdout, stderr, code] = await Promise.all([
    new Response(child.stdout).text(),
    new Response(child.stderr).text(),
    child.exited,
  ]);
  return { code, stdout: stdout.trim(), stderr: stderr.trim() };
}

async function attachedSessionClientCount(parentPID: number): Promise<number> {
  const processes = await run(["ps", "-axo", "ppid=,command="]);
  if (processes.code !== 0) throw new Error(processes.stderr || "could not inspect terminal clients");
  return processes.stdout.split("\n").filter((line) => {
    const match = line.trim().match(/^(\d+)\s+(.+)$/);
    return match && Number(match[1]) === parentPID && match[2]!.includes("zmx attach");
  }).length;
}

async function setupEnvironment() {
  const directory = mkdtempSync(join(tmpdir(), "slis-term-e2e-"));
  const configHome = join(directory, "config");
  const stateHome = join(directory, "state");
  const workspaceRoot = join(directory, "workspace");
  const repo = join(workspaceRoot, "app");
  mkdirSync(repo, { recursive: true });
  await run(["git", "init", "-q", repo]);
  await run(["git", "-C", repo, "config", "user.email", "e2e@example.com"]);
  await run(["git", "-C", repo, "config", "user.name", "Slis E2E"]);
  writeFileSync(join(repo, "README.md"), "e2e\n");
  await run(["git", "-C", repo, "add", "README.md"]);
  await run(["git", "-C", repo, "commit", "-q", "-m", "test"]);
  await run(["git", "-C", repo, "branch", "-M", "main"]);
  await run(["git", "-C", repo, "branch", SLICE]);
  const worktree = join(workspaceRoot, ".slis", "worktrees", SLICE, "app");
  mkdirSync(dirname(worktree), { recursive: true });
  await run(["git", "-C", repo, "worktree", "add", "-q", worktree, SLICE]);
  const slisBinary = process.env.SLIS_BIN ?? join(projectRoot, "slis");
  const runtimeBinary = process.env.SLIS_ZMX_BINARY ?? join(dirname(slisBinary), "zmx");
  if (!existsSync(slisBinary)) throw new Error(`missing Slis binary: ${slisBinary}`);
  if (!existsSync(runtimeBinary)) throw new Error(`missing Slis session runtime: ${runtimeBinary}`);
  const env = {
    ...process.env,
    TERM: "xterm-256color",
    SLIS_FAKE: "1",
    SLIS_BIN: slisBinary,
    SLIS_ZMX_BINARY: runtimeBinary,
    SLIS_SESSION_RUNTIME_DIR: `/tmp/slis-e2e-${process.pid}`,
    XDG_CONFIG_HOME: configHome,
    XDG_STATE_HOME: stateHome,
  };
  const initialized = await run(
    [slisBinary, "init", workspaceRoot, "--repos", "app", "--no-hooks", "--no-skill"],
    env,
  );
  if (initialized.code !== 0) throw new Error(initialized.stderr || initialized.stdout);
  const workspaceConfig = initialized.stdout.match(/^workspace written to: (.+)$/m)?.[1];
  if (!workspaceConfig) throw new Error(`could not read workspace path from: ${initialized.stdout}`);
  const workspaceEnv = { ...env, SLIS_WORKSPACE_CONFIG: workspaceConfig };
  const ensured = await run([slisBinary, "session", "ensure", SLICE], workspaceEnv);
  if (ensured.code !== 0) throw new Error(ensured.stderr || ensured.stdout);
  return { directory, env: workspaceEnv, slisBinary };
}

function spawnAttachedClient(command: string[], env: Record<string, string | undefined>, terminal: PersistentTerminal, cols: number, rows: number): any {
  const responder = new TerminalQueryResponder();
  const pending: string[] = [];
  let child: any;
  child = Bun.spawn(command, {
    env,
    terminal: {
      cols,
      rows,
      data(_terminal: unknown, data: Uint8Array) {
        for (const response of responder.observe(data)) {
          if (child) child.terminal.write(response);
          else pending.push(response);
        }
        terminal.feed(data);
      },
    },
  } as any);
  for (const response of pending) child.terminal.write(response);
  return child;
}

async function main() {
  const fixture = await setupEnvironment();
  const vt = new PersistentTerminal({ cols: APP_COLS, rows: APP_ROWS });
  let tearingDown = false;
  const appCommand = process.env.SLIS_E2E_BIN
    ? [process.env.SLIS_E2E_BIN]
    : ["bun", "run", "src/index.tsx"];
  const app: any = Bun.spawn(appCommand, {
    cwd: tuiRoot,
    env: fixture.env,
    terminal: {
      cols: APP_COLS,
      rows: APP_ROWS,
      data(_terminal: unknown, data: Uint8Array) {
        if (!tearingDown) vt.feed(data);
      },
    },
  } as any);
  const pty = app.terminal;
  await sleep(2800);
  const sawBrowser = vt.getText().includes(SLICE) && vt.getText().toLowerCase().includes("slices");

  if (process.env.SLIS_E2E_QUIT_ONLY === "1") {
    pty.write("c");
    await sleep(300);
    const sawCreate = vt.getText().includes("Create slice");
    pty.write("\x03");
    const quit = await Promise.race([app.exited.then(() => true), sleep(1200).then(() => false)]);
    tearingDown = true;
    app.kill();
    vt.destroy();
    await run([fixture.slisBinary, "session", "kill", SLICE], fixture.env);
    rmSync(fixture.directory, { recursive: true, force: true });
    console.log(`installed_browser=${sawBrowser} create_overlay=${sawCreate} ctrl_c_quit=${quit}`);
    process.exit(sawBrowser && sawCreate && quit ? 0 : 1);
  }

  pty.write("\r");
  await sleep(700);
  const cockpitOpened = vt.getText().includes(`slis › ${SLICE}`);
  pty.write("\x1b[B\x1b[B\x1b[B");
  await sleep(700);
  const breadcrumbSurvivesArrows = vt.getText().includes(`slis › ${SLICE}`);
  pty.write("\x1b");
  await sleep(500);
  pty.write("a");
  await sleep(1400);
  const terminalText = vt.getText();
  const sawTabBar = terminalText.includes("TERM") && terminalText.includes("claude");
  const browserVisibleAlongsideDock = terminalText.includes("FILTERS") && terminalText.includes("CHANGES") && terminalText.includes("SLICES");
  const visibleSessionAttached = await attachedSessionClientCount(app.pid) > 0;
  pty.write("\x11");
  await sleep(5500);
  const backToBrowser = vt.getText().includes("SLICES") && !vt.getText().includes("ctrl+q back");
  const hiddenSessionDetached = await attachedSessionClientCount(app.pid) === 0;
  pty.write("a");
  await sleep(1400);
  const reopenedSessionAttached = await attachedSessionClientCount(app.pid) > 0;

  if (process.env.SLIS_E2E_SUSPEND_ONLY === "1") {
    tearingDown = true;
    app.kill();
    vt.destroy();
    await run([fixture.slisBinary, "session", "kill", SLICE], fixture.env);
    rmSync(fixture.directory, { recursive: true, force: true });
    const hiddenSessionDetaches = visibleSessionAttached && hiddenSessionDetached;
    console.log(`${hiddenSessionDetaches ? "PASS" : "FAIL"} hidden_session_detaches`);
    console.log(`${reopenedSessionAttached ? "PASS" : "FAIL"} reopening_session_reattaches`);
    process.exit(hiddenSessionDetaches && reopenedSessionAttached ? 0 : 1);
  }

  const marker = `SLIS_EMBED_${Date.now()}`;
  pty.write(`printf '${marker}\\n'\r`);
  await sleep(900);
  const sawMarker = vt.getText().includes(marker);
  const newlinePrefix = `SLIS_LINE_${Date.now()}_`;
  pty.write(`printf '${newlinePrefix}A\\n${newlinePrefix}B\\n${newlinePrefix}C\\n'\r`);
  await sleep(900);
  const renderedLines = vt.getText().split("\n").filter((line) => line.includes(newlinePrefix));
  const renderedColumns = ["A", "B", "C"].map((suffix) =>
    renderedLines
      .filter((line) => line.includes(newlinePrefix + suffix))
      .map((line) => line.indexOf(newlinePrefix + suffix)),
  );
  const consecutiveNewlinesStartAtColumnZero = renderedColumns[0]!.some((column) =>
    renderedColumns.slice(1).every((columns) => columns.includes(column)),
  );

  const pastedMarker = `SLIS_PASTE_${Date.now()}`;
  pty.write(`\x1b[200~printf '${pastedMarker}\\n'\x1b[201~`);
  await sleep(200);
  pty.write("\r");
  await sleep(800);
  const pasteReachedTerminal = vt.getText().includes(pastedMarker);
  pty.write("\x03");
  await sleep(250);
  const interruptMarker = `SLIS_AFTER_CTRL_C_${Date.now()}`;
  pty.write(`printf '${interruptMarker}\\n'\r`);
  await sleep(700);
  const ctrlCReachedTerminal = vt.getText().includes(interruptMarker);

  const secondVT = new PersistentTerminal({ cols: 100, rows: 30 });
  const second = spawnAttachedClient([fixture.slisBinary, "session", "attach", SLICE, AGENT_TAB], fixture.env, secondVT, 100, 30);
  await sleep(1000);
  const multiClientMarker = `SLIS_MULTI_${Date.now()}`;
  second.terminal.write(`printf '${multiClientMarker}\\n'\r`);
  await sleep(800);
  const multiClientMirrorsOutput = secondVT.getText().includes(multiClientMarker) && vt.getText().includes(multiClientMarker);
  second.kill();
  await second.exited;
  secondVT.destroy();

  const reconnectVT = new PersistentTerminal({ cols: 90, rows: 24 });
  const reconnect = spawnAttachedClient([fixture.slisBinary, "session", "attach", SLICE, AGENT_TAB], fixture.env, reconnectVT, 90, 24);
  await sleep(1000);
  const reconnectRestoresHistory = reconnectVT.getText().includes(multiClientMarker);
  reconnect.terminal.resize(120, 35);
  const resizeMarker = `SLIS_RESIZE_${Date.now()}`;
  reconnect.terminal.write(`printf '${resizeMarker}\\n'\r`);
  await sleep(700);
  const resizeKeepsTerminalUsable = reconnectVT.getText().includes(resizeMarker);
  reconnect.kill();
  await reconnect.exited;
  reconnectVT.destroy();

  const streamPrefix = "SLIS_STREAM_";
  pty.write(`i=1; while [ "$i" -le 45 ]; do if [ "$i" -gt 1 ]; then printf '\\033[8A'; fi; j=1; while [ "$j" -le 8 ]; do printf '\\r\\033[2K\\033[36m${streamPrefix}%03d_%02d payload\\033[0m\\n' "$i" "$j"; j=$((j+1)); done; sleep 0.04; i=$((i+1)); done\r`);
  const malformedFrames: string[] = [];
  const observedStreamLines = new Set<string>();
  for (let sample = 0; sample < 30; sample++) {
    await sleep(100);
    const sampledStreamLines = vt.getText().split("\n").filter((line) => line.includes(streamPrefix));
    malformedFrames.push(...sampledStreamLines.filter((line) => {
      const start = line.indexOf(streamPrefix);
      if (start < 0 || line.includes("%03d")) return false;
      const terminalLine = line.slice(start);
      if (!new RegExp(`^${streamPrefix}\\d{3}_\\d{2} payload\\s*$`).test(terminalLine)) return true;
      observedStreamLines.add(terminalLine.trim());
      return false;
    }));
  }
  const sustainedOutputClean = observedStreamLines.size > 3 && malformedFrames.length === 0;

  pty.write("\x11");
  await sleep(800);
  pty.write("t");
  await sleep(1400);
  const sawShellTab = vt.getText().includes("shell");
  const agentHistory = await run([fixture.slisBinary, "session", "history", SLICE, AGENT_TAB], fixture.env);
  const shellHistory = await run([fixture.slisBinary, "session", "history", SLICE, "shell"], fixture.env);
  const pasteExecutes = pasteReachedTerminal && agentHistory.stdout.split("\n").includes(pastedMarker);
  const bothTabsAlive = agentHistory.code === 0 && shellHistory.code === 0;
  pty.write("\x11");
  await sleep(500);
  pty.write("c");
  await sleep(300);
  const createOverlayOpen = vt.getText().includes("Create slice");
  pty.write("\x03");
  const ctrlCQuitsBrowser = await Promise.race([app.exited.then(() => true), sleep(1200).then(() => false)]);
  const persistentHistory = await run([fixture.slisBinary, "session", "history", SLICE, AGENT_TAB], fixture.env);
  const sessionSurvivesAppQuit = persistentHistory.code === 0 && persistentHistory.stdout.includes(marker);

  tearingDown = true;
  app.kill();
  const lastPaint = vt.getText();
  vt.destroy();
  await run([fixture.slisBinary, "session", "kill", SLICE], fixture.env);
  rmSync(fixture.directory, { recursive: true, force: true });
  const results: Record<string, boolean> = {
    browser_paints_slice_list: sawBrowser,
    enter_opens_cockpit: cockpitOpened,
    breadcrumb_survives_arrow_navigation: breadcrumbSurvivesArrows,
    key_a_opens_terminal_tab: sawTabBar,
    terminal_dock_keeps_browser_visible: browserVisibleAlongsideDock,
    keystrokes_reach_embedded_shell: sawMarker,
    consecutive_newlines_start_at_column_zero: consecutiveNewlinesStartAtColumnZero,
    paste_reaches_embedded_shell: pasteExecutes,
    ctrl_c_reaches_embedded_terminal: ctrlCReachedTerminal,
    multiple_clients_mirror_output: multiClientMirrorsOutput,
    reconnect_restores_history: reconnectRestoresHistory,
    resize_keeps_terminal_usable: resizeKeepsTerminalUsable,
    sustained_terminal_output_stays_clean: sustainedOutputClean,
    ctrl_q_returns_to_browser: backToBrowser,
    hidden_session_detaches: visibleSessionAttached && hiddenSessionDetached,
    reopening_session_reattaches: reopenedSessionAttached,
    shell_tab_is_separate: sawShellTab && bothTabsAlive,
    c_opens_create_overlay: createOverlayOpen,
    ctrl_c_quits_from_overlay: ctrlCQuitsBrowser,
    session_survives_app_quit: sessionSurvivesAppQuit,
  };
  for (const [name, passed] of Object.entries(results)) console.log(`${passed ? "PASS" : "FAIL"} ${name}`);
  if (!Object.values(results).every(Boolean)) {
    console.log("TERMINAL\n" + terminalText.split("\n").slice(0, 30).join("\n"));
    console.log("AGENT HISTORY\n" + agentHistory.stdout);
    console.log("SHELL HISTORY\n" + shellHistory.stdout);
    console.log("MALFORMED STREAM FRAMES\n" + malformedFrames.slice(0, 20).join("\n"));
    console.log("LAST PAINT\n" + lastPaint.split("\n").slice(0, 30).join("\n"));
  }
  process.exit(Object.values(results).every(Boolean) ? 0 : 1);
}

main().catch((error) => {
  console.error("[term e2e] ERROR", error);
  process.exit(1);
});
