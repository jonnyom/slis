import { useEffect, useState } from "react";

export const HIDDEN_SESSION_DETACH_DELAY_MS = 5_000;

export function useDelayedSessionAttachment(
  shown: boolean,
  suspendable: boolean,
  delayMs = HIDDEN_SESSION_DETACH_DELAY_MS,
): boolean {
  const [attached, setAttached] = useState(shown || !suspendable);

  useEffect(() => {
    if (!suspendable || shown) {
      setAttached(true);
      return;
    }
    const timer = setTimeout(() => setAttached(false), delayMs);
    return () => clearTimeout(timer);
  }, [delayMs, shown, suspendable]);

  return attached;
}
