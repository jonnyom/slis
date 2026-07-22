// Adopt an arbitrary branch as a slice (parity gap D3). The Go browser's `I`
// runs an interactive adopt of any branch; here it prompts for the branch name
// then routes through the PTY-tab path (adopt is an interactive command). The
// text-input card mirrors CreateOverlay's shape.

import type { ReactNode } from "react";
import { Card } from "../components/card";
import { TextField } from "../components/textfield";

export function AdoptOverlay({ text }: { text: string }): ReactNode {
  return (
    <Card
      title="Adopt branch"
      subtitle="Import any branch as a managed slice (worktree per repo where it exists)."
      width={60}
      hints={[
        { key: "enter", label: "adopt" },
        { key: "esc", label: "cancel" },
      ]}
    >
      <TextField id="adopt-branch-name" label="Branch name" lines={[text]} />
    </Card>
  );
}
