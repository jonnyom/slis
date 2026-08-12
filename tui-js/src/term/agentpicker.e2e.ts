import { PersistentTerminal } from "ghostty-opentui";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";

const APP_COLS = 160;
const APP_ROWS = 44;
const SLICE = "checkout";
const sleep = (milliseconds: number) => new Promise((resolveSleep) => setTimeout(resolveSleep, milliseconds));
const projectRoot = resolve(import.meta.dir, "../../..");
const tuiRoot = resolve(import.meta.dir, "../..");

async function run(command: string[], env: Record<string, string | undefined> = process.env) {
  const child = Bun.spawn(command, { stdout: "pipe", stderr: "pipe", stdin: "ignore", env });
  const timer = setTimeout(() => child.kill(), 10_000);
  const [stdout, stderr, code] = await Promise.all([new Response(child.stdout).text(), new Response(child.stderr).text(), child.exited]);
  clearTimeout(timer);
  return { code, stdout: stdout.trim(), stderr: stderr.trim() };
}

async function setupEnvironment() {
  const directory = mkdtempSync(join(tmpdir(), "slis-picker-e2e-"));
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
  mkdirSync(join(configHome, "slis"), { recursive: true });
  writeFileSync(join(configHome, "slis", "workspace.yaml"), `root: ${JSON.stringify(workspaceRoot)}\nrepos:\n  app:\n    primary: ${JSON.stringify(repo)}\ngrouping:\n  strategy: branch-name\nsessions:\n  layout: root\n`);
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
    SLIS_SESSION_RUNTIME_DIR: `/tmp/slis-picker-${process.pid}`,
    XDG_CONFIG_HOME: configHome,
    XDG_STATE_HOME: stateHome,
  };
  const ensured = await run([slisBinary, "session", "ensure", SLICE], env);
  if (ensured.code !== 0) throw new Error(ensured.stderr || ensured.stdout);
  return { directory, env, slisBinary };
}

async function main() {
  if (Bun.which("codex") === null) throw new Error("codex is required for agent rendering proof");
  const fixture = await setupEnvironment();
  const vt = new PersistentTerminal({ cols: APP_COLS, rows: APP_ROWS });
  let tearingDown = false;
  const app: any = Bun.spawn(["bun", "run", "src/index.tsx"], {
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
  pty.write(",");
  await sleep(700);
  const configText = vt.getText();
  const sawConfig = configText.includes("Agent settings") && configText.includes("set default") && !configText.includes("make default");
  pty.write("\x1b");
  await sleep(300);
  pty.write("C");
  await sleep(900);
  const pickerText = vt.getText();
  const sawPicker = pickerText.includes("Launch which agent?") && pickerText.includes("claude") && pickerText.includes("codex");
  pty.write("2");
  await sleep(2500);
  const tabText = vt.getText();
  const pickerGone = !tabText.includes("Launch which agent?");
  const sawAgentTab = tabText.includes(SLICE) && tabText.includes("codex");
  const busy = await run([fixture.slisBinary, "session", "busy", SLICE, "agent"], fixture.env);
  const realCodexRuns = busy.code === 0 && JSON.parse(busy.stdout).busy === true;
  const visibleLines = tabText.split("\n");
  const codexLines = visibleLines.filter((line) => /codex|openai|model/i.test(line));
  const sawTrustPromptLayout = visibleLines.some((line) => line.includes("Do you trust the contents")) && visibleLines.some((line) => line.includes("1. Yes, continue")) && visibleLines.some((line) => line.includes("2. No, quit"));
  const codexRendersReadableLines = (sawTrustPromptLayout || codexLines.length >= 2) && visibleLines.every((line) => !/^\s{40,}\S/.test(line));
  pty.write("\x11");
  await sleep(500);
  const browserDockText = vt.getText();
  const agentDockPersistsInBrowser = browserDockText.includes("FILTERS") && browserDockText.includes("term") && browserDockText.includes(SLICE);
  pty.write("j");
  await sleep(500);
  const otherSliceText = vt.getText();
  const sliceWithoutAgentHidesDock = otherSliceText.includes("FILTERS") && !otherSliceText.includes(" term ");
  pty.write("k");
  await sleep(500);
  const returnedSliceText = vt.getText();
  const returningToSliceRestoresDock = returnedSliceText.includes("FILTERS") && returnedSliceText.includes(" term ") && returnedSliceText.includes(SLICE);
  pty.write(",");
  await sleep(700);
  const dockedOverlayText = vt.getText();
  const popupSuppressesAgentDock = dockedOverlayText.includes("Agent settings") && !dockedOverlayText.includes(" term ");
  const popupBusy = await run([fixture.slisBinary, "session", "busy", SLICE, "agent"], fixture.env);
  const popupKeepsAgentRunning = popupBusy.code === 0 && JSON.parse(popupBusy.stdout).busy === true;
  pty.write("\x1b");
  await sleep(500);
  const restoredDockText = vt.getText();
  const closingPopupRestoresAgentDock = restoredDockText.includes("FILTERS") && restoredDockText.includes(" term ");
  pty.write("a");
  await sleep(500);
  const hiddenDockText = vt.getText();
  const aHidesAgentDock = hiddenDockText.includes("FILTERS") && !hiddenDockText.includes(" term ");
  const hiddenBusy = await run([fixture.slisBinary, "session", "busy", SLICE, "agent"], fixture.env);
  const hiddenDockKeepsAgentRunning = hiddenBusy.code === 0 && JSON.parse(hiddenBusy.stdout).busy === true;
  pty.write("a");
  await sleep(600);
  pty.write("\x11");
  await sleep(400);
  pty.write("L");
  await sleep(700);
  const alternatePickerText = vt.getText();
  const LOpensPickerWithSavedDefault = alternatePickerText.includes("Launch which agent?");
  pty.write("1");
  await sleep(1800);
  const alternateSessions = await run([fixture.slisBinary, "session", "list"], fixture.env);
  const alternateGroups = alternateSessions.code === 0 ? JSON.parse(alternateSessions.stdout) as Array<{ tabs: Array<{ id: string }> }> : [];
  const alternateAgentGetsSeparateTab = alternateGroups.some((group) => group.tabs.some((tab) => tab.id.startsWith("agent-")));
  pty.write("\x11");
  await sleep(400);
  pty.write("C");
  await sleep(800);
  const repeatText = vt.getText();
  const defaultSessions = await run([fixture.slisBinary, "session", "list"], fixture.env);
  const defaultGroups = defaultSessions.code === 0 ? JSON.parse(defaultSessions.stdout) as Array<{ active_tab_id: string }> : [];
  const savedDefaultSkipsPicker = !repeatText.includes("Launch which agent?") && defaultGroups.some((group) => group.active_tab_id === "agent");
  pty.write("\x11");
  await sleep(400);
  pty.write("\r");
  await sleep(700);
  const cockpitDockText = vt.getText();
  const agentDockPersistsInCockpit = cockpitDockText.includes("REPOS & STACK") && cockpitDockText.includes("term") && cockpitDockText.includes(SLICE);
  pty.write("\r");
  await sleep(700);
  const diffDockText = vt.getText();
  const agentDockPersistsInDiff = diffDockText.includes("term") && diffDockText.includes(SLICE) && !diffDockText.includes("FILTERS");
  const lastPaint = vt.getText();
  tearingDown = true;
  app.kill();
  await Promise.race([app.exited, sleep(2000)]);
  vt.destroy();
  await run([fixture.slisBinary, "session", "kill", SLICE], fixture.env);
  rmSync(fixture.directory, { recursive: true, force: true });
  const results: Record<string, boolean> = {
    comma_opens_agent_configuration: sawConfig,
    C_opens_agent_picker_with_both_agents: sawPicker,
    quick_pick_dismisses_picker: pickerGone,
    session_tab_titled_with_agent_name: sawAgentTab,
    real_codex_process_runs: realCodexRuns,
    codex_renders_readable_lines: codexRendersReadableLines,
    saved_default_skips_picker: savedDefaultSkipsPicker,
    agent_dock_persists_in_browser: agentDockPersistsInBrowser,
    slice_without_agent_hides_dock: sliceWithoutAgentHidesDock,
    returning_to_slice_restores_dock: returningToSliceRestoresDock,
    popup_suppresses_agent_dock: popupSuppressesAgentDock,
    popup_keeps_agent_running: popupKeepsAgentRunning,
    closing_popup_restores_agent_dock: closingPopupRestoresAgentDock,
    a_hides_agent_dock: aHidesAgentDock,
    hidden_dock_keeps_agent_running: hiddenDockKeepsAgentRunning,
    L_opens_picker_with_saved_default: LOpensPickerWithSavedDefault,
    alternate_agent_gets_separate_tab: alternateAgentGetsSeparateTab,
    agent_dock_persists_in_cockpit: agentDockPersistsInCockpit,
    agent_dock_persists_in_diff: agentDockPersistsInDiff,
  };
  for (const [name, passed] of Object.entries(results)) console.log(`${passed ? "PASS" : "FAIL"} ${name}`);
  if (!Object.values(results).every(Boolean)) console.log(lastPaint.split("\n").slice(0, 30).join("\n"));
  process.exit(Object.values(results).every(Boolean) ? 0 : 1);
}

main().catch((error) => {
  console.error("[agent-picker e2e] ERROR", error);
  process.exit(1);
});
