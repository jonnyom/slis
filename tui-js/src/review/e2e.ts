// End-to-end proof for the inline-review loop (F2): run the REAL app
// (src/index.tsx) in a Bun PTY with the fake sidecar, drive the whole loop —
// open the rich diff → focus lines → select a range → `c` compose →
// submit → `C` list it → `s`/`y`
// send → toast — and read what it actually paints (via ghostty's parser). No
// human, no real `slis` binary (SLIS_FAKE drives the shared fake review store).
//
// Runs at two sizes so layout is proven narrow and wide.
// Run: bun run src/review/e2e.ts

import { PersistentTerminal } from "ghostty-opentui";

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

interface Size {
  cols: number;
  rows: number;
}

async function driveOnce(size: Size): Promise<Record<string, boolean>> {
  const vt = new PersistentTerminal({ cols: size.cols, rows: size.rows });
  const app: any = Bun.spawn(["bun", "run", "src/index.tsx"], {
    cwd: `${import.meta.dir}/../..`,
    env: { ...process.env, TERM: "xterm-256color", SLIS_FAKE: "1" },
    terminal: {
      cols: size.cols,
      rows: size.rows,
      data(_t: unknown, data: Uint8Array) {
        vt.feed(data);
      },
    },
  } as any);
  const pty = app.terminal;

  await sleep(2800); // boot: renderer + fake data + first paint
  const boot = vt.getText();
  const sawBrowser = boot.includes("checkout") && boot.toLowerCase().includes("slices");

  // Open the focused slice (checkout) cockpit, then the rich diff.
  pty.write("l");
  await sleep(900);
  const cockpit = vt.getText();
  const sawBadge = cockpit.includes("✎"); // seed comment → breadcrumb ✎ 1

  pty.write("\r"); // enter → rich diff (member branch)
  await sleep(900);
  const diff = vt.getText();
  const sawDiff = diff.includes("cart.tsx");
  const sawGutterMarker = diff.includes("✎"); // seed comment on cart.tsx line 12

  // c focuses diff lines; v starts a range; j extends it; c opens composer.
  pty.write("c");
  await sleep(300);
  pty.write("vj");
  await sleep(300);
  pty.write("c");
  await sleep(700);
  const composer = vt.getText();
  const sawComposer =
    composer.includes("Comment on selected lines") &&
    composer.includes("comment for the agent");

  pty.write("rename this variable");
  await sleep(400);
  pty.write("\r");
  await sleep(900);
  const afterAdd = vt.getText();
  const sawAddToast = afterAdd.includes("Added review comment");

  // C → pending-review overlay lists the comment(s).
  pty.write("C");
  await sleep(900);
  const reviewList = vt.getText();
  const sawList =
    reviewList.toLowerCase().includes("pending") &&
    (reviewList.includes("rename this variable") || reviewList.includes("cart.tsx"));

  // s → send confirm → y → send; success toast.
  pty.write("s");
  await sleep(600);
  const confirm = vt.getText();
  const sawConfirm = confirm.includes("agent session");

  pty.write("y");
  await sleep(900);
  const afterSend = vt.getText();
  const sawSendToast = afterSend.includes("Sent");

  // esc chain: send closes overlay; esc leaves line focus, closes diff, then
  // returns from cockpit to browser.
  pty.write("\x1b");
  await sleep(300);
  pty.write("\x1b");
  await sleep(300);
  pty.write("\x1b");
  await sleep(700);
  const back = vt.getText();
  const escChain = back.toLowerCase().includes("slices");

  pty.write("q");
  await sleep(600);
  try {
    app.kill();
  } catch {
    // best-effort teardown
  }
  const last = vt.getText();
  vt.destroy();

  const R: Record<string, boolean> = {
    browser_paints_slice_list: sawBrowser,
    cockpit_breadcrumb_comment_badge: sawBadge,
    rich_diff_opens: sawDiff,
    gutter_marker_visible: sawGutterMarker,
    range_opens_comment_composer: sawComposer,
    submit_shows_add_toast: sawAddToast,
    C_lists_pending_comments: sawList,
    s_shows_send_confirm: sawConfirm,
    y_sends_shows_toast: sawSendToast,
    esc_chain_returns_to_browser: escChain,
  };
  if (!Object.values(R).every(Boolean)) {
    console.log(`\n--- last paint @ ${size.cols}x${size.rows} (first 20 lines) ---`);
    console.log(last.split("\n").slice(0, 20).join("\n"));
  }
  return R;
}

async function main() {
  const sizes: Size[] = [
    { cols: 120, rows: 35 },
    { cols: 160, rows: 42 },
  ];
  let ok = true;
  for (const size of sizes) {
    const R = await driveOnce(size);
    console.log(`\n===== review-loop E2E @ ${size.cols}x${size.rows} =====`);
    for (const [k, v] of Object.entries(R)) {
      console.log(`  ${(v ? "PASS" : "FAIL").padEnd(5)} ${k}`);
      if (!v) ok = false;
    }
  }
  console.log("\n" + (ok ? "ALL PASS" : "SOME FAILED"));
  process.exit(ok ? 0 : 1);
}

main().catch((e) => {
  console.error("[review e2e] ERROR", e);
  process.exit(1);
});
