import { describe, expect, test } from "bun:test";
import type { ReactElement } from "react";
import { TerminalLink } from "./terminal-link";

describe("TerminalLink", () => {
  test("renders a selectable OpenTUI hyperlink with its URL visible", () => {
    const url = "https://github.com/Noryai/nory/pull/8584";
    const outer = TerminalLink({ url }) as ReactElement<{
      selectable: boolean;
      children: ReactElement<{ href: string; children: string }>;
    }>;
    expect(outer.type).toBe("text");
    expect(outer.props.selectable).toBe(true);
    expect(outer.props.children.type).toBe("a");
    expect(outer.props.children.props.href).toBe(url);
    expect(outer.props.children.props.children).toBe(url);
  });
});
