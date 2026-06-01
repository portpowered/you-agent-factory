const GO_DURATION_FRAGMENT = /^(-?(?:\d+(?:\.\d+)?|\.\d+))([µμ]?s|ms|m|h)/;

const GO_DURATION_UNIT_TO_NANOSECONDS: Record<string, number> = {
  ns: 1,
  us: 1_000,
  µs: 1_000,
  μs: 1_000,
  ms: 1_000_000,
  s: 1_000_000_000,
  m: 60 * 1_000_000_000,
  h: 60 * 60 * 1_000_000_000,
};

/** Parses a Go `time.ParseDuration` string into nanoseconds, or null when invalid. */
export function parseGoDurationNanoseconds(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed.length === 0) {
    return null;
  }

  let remaining = trimmed;
  let totalNanoseconds = 0;
  let sawFragment = false;

  while (remaining.length > 0) {
    const match = remaining.match(GO_DURATION_FRAGMENT);
    if (!match) {
      return null;
    }

    sawFragment = true;
    const amount = Number.parseFloat(match[1]);
    if (!Number.isFinite(amount)) {
      return null;
    }

    const unitNanoseconds = GO_DURATION_UNIT_TO_NANOSECONDS[match[2]];
    if (unitNanoseconds === undefined) {
      return null;
    }

    totalNanoseconds += amount * unitNanoseconds;
    remaining = remaining.slice(match[0].length);
  }

  return sawFragment ? totalNanoseconds : null;
}
