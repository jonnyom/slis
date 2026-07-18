import { useKeyboard } from "@opentui/react";
import { Component, type ErrorInfo, type ReactNode } from "react";
import { normalizeKeyName } from "../util/keys";
import { theme } from "../theme";
import { BOLD, DIM } from "./ui";

function CrashScreen({ error, onQuit }: { error: Error; onQuit: () => void }): ReactNode {
  useKeyboard((key) => {
    const name = normalizeKeyName(key);
    if (name === "q" || name === "escape" || (key.ctrl && name === "c")) onQuit();
  });

  const details = (error.stack || String(error)).split("\n").slice(0, 14);
  return (
    <box width="100%" height="100%" flexDirection="column" padding={1}>
      <text fg={theme.bad} attributes={BOLD}>Slis hit a UI error</text>
      <text fg={theme.text} wrapMode="word">Press q, Esc, or Ctrl+C to quit safely.</text>
      <box marginTop={1} flexDirection="column">
        {details.map((line, index) => (
          <text key={index} fg={theme.textDim} attributes={DIM} wrapMode="word">
            {line}
          </text>
        ))}
      </box>
    </box>
  );
}

export class ErrorBoundary extends Component<
  { children: ReactNode; onQuit: () => void },
  { error: Error | null }
> {
  override state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error): { error: Error } {
    return { error };
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("Slis UI render error", error, info.componentStack);
  }

  override render(): ReactNode {
    return this.state.error ? (
      <CrashScreen error={this.state.error} onQuit={this.props.onQuit} />
    ) : (
      this.props.children
    );
  }
}
