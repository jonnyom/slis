import { describe, expect, test } from "bun:test";

import {
  sessionAttachArgv,
  sessionActivateTabArgv,
  sessionBusyArgv,
  sessionEnsureTabArgv,
  sessionLegacyAttachArgv,
  sessionLegacyKillArgv,
  sessionLegacyListArgv,
  sessionLegacySendArgv,
  sessionListArgv,
  sessionKillArgv,
  sessionKillTabArgv,
  sessionSendArgv,
  sessionStartArgv,
} from "./slis";

describe("Slis terminal commands", () => {
  test("routes attach and input through Slis", () => {
    expect(sessionAttachArgv("feature", "repo-api")).toEqual([
      "slis",
      "session",
      "attach",
      "feature",
      "repo-api",
    ]);
    expect(sessionActivateTabArgv("feature", "repo-api")).toEqual([
      "slis",
      "session",
      "activate-tab",
      "feature",
      "repo-api",
    ]);
    expect(sessionSendArgv("feature", "repo-api")).toEqual([
      "slis",
      "session",
      "send",
      "feature",
      "repo-api",
    ]);
    expect(sessionStartArgv("feature", "agent")).toEqual([
      "slis",
      "session",
      "start",
      "feature",
      "agent",
    ]);
    expect(sessionEnsureTabArgv("feature", "agent", "agent", "Codex")).toEqual([
      "slis",
      "session",
      "ensure-tab",
      "feature",
      "agent",
      "--kind",
      "agent",
      "--title",
      "Codex",
    ]);
    expect(sessionBusyArgv("feature", "agent")).toEqual([
      "slis",
      "session",
      "busy",
      "feature",
      "agent",
    ]);
    expect(sessionLegacyAttachArgv("slis/feature")).toEqual([
      "slis",
      "session",
      "legacy-attach",
      "slis/feature",
    ]);
    expect(sessionLegacyListArgv()).toEqual(["slis", "session", "legacy-list"]);
    expect(sessionLegacyKillArgv("slis/feature")).toEqual([
      "slis",
      "session",
      "legacy-kill",
      "slis/feature",
    ]);
    expect(sessionLegacySendArgv("slis/feature")).toEqual([
      "slis",
      "session",
      "legacy-send",
      "slis/feature",
    ]);
    expect(sessionListArgv()).toEqual(["slis", "session", "list"]);
    expect(sessionKillArgv("feature")).toEqual(["slis", "session", "kill", "feature"]);
    expect(sessionKillTabArgv("feature", "agent-2")).toEqual([
      "slis",
      "session",
      "kill-tab",
      "feature",
      "agent-2",
    ]);
  });
});
