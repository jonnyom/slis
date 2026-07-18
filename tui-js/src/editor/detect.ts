// Editor detection, ported from internal/editor/editor.go. The known editors and
// their detection-preference order match the Go side exactly, so the JS TUI's
// e/o keys resolve to the same editor `slis edit` would pick. Kept pure (the PATH
// probe is injected) so it is unit-testable without touching the real PATH.

export interface EditorSpec {
  name: string;
  bin: string;
}

// Detection-preference order, mirroring editor.go's `known` list.
export const KNOWN_EDITORS: readonly EditorSpec[] = [
  { name: "Cursor", bin: "cursor" },
  { name: "VS Code", bin: "code" },
  { name: "VS Code Insiders", bin: "code-insiders" },
  { name: "VSCodium", bin: "codium" },
  { name: "Windsurf", bin: "windsurf" },
  { name: "Zed", bin: "zed" },
];

// availableEditors returns the known editors found on PATH, in preference order.
// `has(bin)` is the PATH probe (e.g. `(b) => !!Bun.which(b)`), injected for tests.
export function availableEditors(has: (bin: string) => boolean): EditorSpec[] {
  return KNOWN_EDITORS.filter((e) => has(e.bin));
}
