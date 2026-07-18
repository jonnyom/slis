type Version = readonly [major: number, minor: number, patch: number];

export function parseVersion(value: string): Version | null {
  const match = /^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$/.exec(value);
  if (!match) return null;
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

export function versionAtLeast(actual: string, required: string): boolean {
  const actualParts = parseVersion(actual);
  const requiredParts = parseVersion(required);
  if (!actualParts || !requiredParts) return false;

  for (let i = 0; i < actualParts.length; i += 1) {
    if (actualParts[i] !== requiredParts[i]) {
      return actualParts[i] > requiredParts[i];
    }
  }
  return true;
}

if (import.meta.main) {
  const required = process.argv[2];
  const actual = Bun.version;
  if (!required || !versionAtLeast(actual, required)) {
    console.error(
      `slis-ui requires Bun ${required ?? "<missing>"} or newer to compile (found ${actual}). ` +
        "Older Bun versions produce a binary that crashes while loading OpenTUI assets.",
    );
    process.exit(1);
  }
}
