import { useKeyboard } from "@opentui/react";
import type { ReactNode } from "react";
import { theme } from "../theme";
import { normalizeKeyName } from "../util/keys";
import { Card } from "./card";
import { BOLD, DIM } from "./ui";

const RECOVERY_INSTRUCTION = "run `slis init` first";

function rawWorkspaceDiagnosis(error: string): string {
  const instructionAt = error.indexOf(RECOVERY_INSTRUCTION);
  if (instructionAt < 0) return error.trim();
  return error
    .slice(instructionAt + RECOVERY_INSTRUCTION.length)
    .replace(/^[\s:—-]+/, "")
    .trim();
}

export function workspaceConfigPath(error: string): string | null {
  return rawWorkspaceDiagnosis(error).match(
    /read "([^"]+)".*no such file or directory/i,
  )?.[1] ?? null;
}

export function workspaceDiagnosis(error: string): string {
  const diagnosis = rawWorkspaceDiagnosis(error);
  const missingPath = workspaceConfigPath(error);
  if (missingPath) {
    return `Slis looked for the workspace configuration at ${missingPath}, but no file exists there.`;
  }
  return diagnosis || error.trim();
}

export function InitialScreen({
  connected,
  error,
  onQuit,
}: {
  connected: boolean;
  error: string | null;
  onQuit: () => void;
}): ReactNode {
  const missingPath = error ? workspaceConfigPath(error) : null;

  useKeyboard((key) => {
    if (!error) return;
    const name = normalizeKeyName(key);
    if (name === "q" || name === "escape" || (key.ctrl && name === "c")) onQuit();
  });

  return (
    <box
      width="100%"
      height="100%"
      alignItems="center"
      justifyContent="center"
      backgroundColor={theme.bg}
    >
      {error ? (
        <Card
          id="initial-workspace-error"
          title="× Workspace unavailable"
          width={72}
          scrim={false}
          borderColor={theme.bad}
          titleColor={theme.bad}
          backgroundColor={theme.bg}
          paddingTop={0}
          paddingBottom={0}
        >
          <text
            id="initial-workspace-heading"
            fg={theme.textBright}
            attributes={BOLD}
            wrapMode="word"
          >
            Workspace not found
          </text>
          <text id="initial-workspace-instruction" fg={theme.text} wrapMode="word">
            {RECOVERY_INSTRUCTION}
          </text>
          <box marginTop={1} flexDirection="column">
            <text id="initial-workspace-diagnosis" fg={theme.textDim} wrapMode="word">
              {missingPath ? (
                <>
                  <span>Slis looked for the workspace configuration at </span>
                  <span fg={theme.text}>{missingPath}</span>
                  <span>, but no file exists there.</span>
                </>
              ) : workspaceDiagnosis(error)}
            </text>
          </box>
          <box id="initial-workspace-footer" marginTop={1}>
            <text wrapMode="none">
              <span fg={theme.focus} attributes={BOLD}>q</span>
              <span fg={theme.textDim}> quit</span>
              <span>{"   "}</span>
              <span fg={theme.focus} attributes={BOLD}>esc</span>
              <span fg={theme.textDim}> quit</span>
              <span>{"   "}</span>
              <span fg={theme.focus} attributes={BOLD}>ctrl+c</span>
              <span fg={theme.textDim}> quit</span>
            </text>
          </box>
        </Card>
      ) : (
        <text fg={theme.textDim} attributes={DIM}>
          {connected ? "loading workspace…" : "connecting to slis rpc…"}
        </text>
      )}
    </box>
  );
}
