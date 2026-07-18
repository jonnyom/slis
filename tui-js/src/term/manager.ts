// TermManager: owns one TermSession per slice so a tab's PTY survives tab
// switches (the React tabs stay mounted; switching just toggles visibility) and
// so app quit can detach every client at once. It never creates or kills tmux
// sessions itself — that lives in TermSession/tmux.ts.

import { TermSession } from "./session";

export class TermManager {
  private readonly sessions = new Map<string, TermSession>();

  /** Get-or-create the (not-yet-attached) session for a slice. */
  session(slice: string): TermSession {
    let s = this.sessions.get(slice);
    if (!s) {
      s = new TermSession(slice);
      this.sessions.set(slice, s);
    }
    return s;
  }

  has(slice: string): boolean {
    return this.sessions.has(slice);
  }

  /** Detach + forget a single slice's client (session survives). */
  detach(slice: string): void {
    const s = this.sessions.get(slice);
    if (!s) return;
    s.detach();
    this.sessions.delete(slice);
  }

  /** Detach every attached client. Called on app quit — sessions survive. */
  detachAll(): void {
    for (const s of this.sessions.values()) s.detach();
    this.sessions.clear();
  }
}
