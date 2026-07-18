// TermManager: owns one TermSession per slice so a tab's PTY survives tab
// switches (the React tabs stay mounted; switching just toggles visibility) and
// so app quit can detach every client at once. It never creates or kills tmux
// sessions itself — that lives in TermSession/tmux.ts.
//
// It also owns CommandSession tabs (interactive one-shot mutations run in a
// PTY). Both kinds are keyed by a tab id — a slice name for tmux sessions, a
// unique id for command tabs — and looked up together via `get`.

import { TermSession } from "./session";
import { CommandSession } from "./command";

export class TermManager {
  private readonly sessions = new Map<string, TermSession>();
  private readonly commands = new Map<string, CommandSession>();

  /** Get-or-create a (not-yet-attached) agent or shell tmux client. */
  session(key: string, slice: string): TermSession {
    let s = this.sessions.get(key);
    if (!s) {
      s = new TermSession(slice);
      this.sessions.set(key, s);
    }
    return s;
  }

  /** Get-or-create the command session for a command tab id. */
  command(id: string, title: string, argv: string[], cwd?: string, env?: Record<string, string>): CommandSession {
    let c = this.commands.get(id);
    if (!c) {
      c = new CommandSession(id, title, argv, cwd, env);
      this.commands.set(id, c);
    }
    return c;
  }

  /** The attachable backing a tab id, whichever kind it is. */
  get(key: string): TermSession | CommandSession | undefined {
    return this.sessions.get(key) ?? this.commands.get(key);
  }

  has(slice: string): boolean {
    return this.sessions.has(slice);
  }

  /** Detach + forget a single tab (tmux session survives; a command is killed). */
  detach(key: string): void {
    const s = this.sessions.get(key);
    if (s) {
      s.detach();
      this.sessions.delete(key);
      return;
    }
    const c = this.commands.get(key);
    if (c) {
      c.detach();
      this.commands.delete(key);
    }
  }

  /** Detach every tab. Called on app quit — tmux sessions survive, commands die. */
  detachAll(): void {
    for (const s of this.sessions.values()) s.detach();
    for (const c of this.commands.values()) c.detach();
    this.sessions.clear();
    this.commands.clear();
  }
}
