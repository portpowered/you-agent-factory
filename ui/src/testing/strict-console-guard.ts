// This helper runs under both Bun and Vitest. Each component setup installs
// these hooks globally; type them without initializing either runner here.
declare const afterEach: typeof import("vitest").afterEach;
declare const beforeEach: typeof import("vitest").beforeEach;

export type ConsoleLevel = "warn" | "error";

export type ConsoleAllowlistEntry = {
  /** Stable identifier referenced in failure output. */
  name: string;
  level: ConsoleLevel;
  /** Substring or narrow RegExp match against the formatted console message. */
  match: string | RegExp;
  /** Why this message is expected in the guarded suite. */
  reason: string;
};

export type StrictConsoleGuardOptions = {
  allowlist?: readonly ConsoleAllowlistEntry[];
};

type ConsoleViolation = {
  level: ConsoleLevel;
  message: string;
};

type GuardState = {
  allowlist: readonly ConsoleAllowlistEntry[];
  violations: ConsoleViolation[];
  originalWarn: typeof console.warn;
  originalError: typeof console.error;
};

let activeGuard: GuardState | undefined;

function formatConsoleArgs(args: unknown[]): string {
  return args
    .map((value) => {
      if (typeof value === "string") {
        return value;
      }
      if (value instanceof Error) {
        return value.message;
      }
      try {
        return JSON.stringify(value);
      } catch {
        return String(value);
      }
    })
    .join(" ");
}

function assertAllowlistEntryIsNarrow(entry: ConsoleAllowlistEntry): void {
  if (entry.name.trim().length === 0) {
    throw new Error("Console allowlist entries require a non-empty name.");
  }
  if (entry.reason.trim().length === 0) {
    throw new Error(
      `Console allowlist entry "${entry.name}" requires a reason.`,
    );
  }

  if (typeof entry.match === "string") {
    const trimmed = entry.match.trim();
    if (trimmed.length < 3) {
      throw new Error(
        `Console allowlist entry "${entry.name}" must use a specific substring match (at least 3 characters).`,
      );
    }
    return;
  }

  const source = entry.match.source;
  if (
    source === ".*" ||
    source === ".+" ||
    source === "^.*$" ||
    source === "^.+$"
  ) {
    throw new Error(
      `Console allowlist entry "${entry.name}" uses a broad wildcard RegExp; use a narrow pattern instead.`,
    );
  }
}

function messageMatchesAllowlistEntry(
  message: string,
  entry: ConsoleAllowlistEntry,
): boolean {
  if (typeof entry.match === "string") {
    return message.includes(entry.match);
  }

  return entry.match.test(message);
}

function isAllowlisted(
  level: ConsoleLevel,
  message: string,
  allowlist: readonly ConsoleAllowlistEntry[],
): boolean {
  return allowlist.some(
    (entry) =>
      entry.level === level && messageMatchesAllowlistEntry(message, entry),
  );
}

function recordConsoleCall(
  level: ConsoleLevel,
  args: unknown[],
  state: GuardState,
): void {
  const message = formatConsoleArgs(args);
  if (isAllowlisted(level, message, state.allowlist)) {
    return;
  }

  state.violations.push({ level, message });
}

function formatViolations(violations: readonly ConsoleViolation[]): string {
  return violations
    .map(
      (violation, index) =>
        `${index + 1}. [console.${violation.level}] ${violation.message}`,
    )
    .join("\n");
}

/** Installs console hooks that record unallowlisted warn/error calls until disposed. */
export function installStrictConsoleGuard(
  options: StrictConsoleGuardOptions = {},
): () => void {
  if (activeGuard) {
    throw new Error(
      "Strict console guard is already installed in this test worker.",
    );
  }

  const allowlist = options.allowlist ?? [];
  for (const entry of allowlist) {
    assertAllowlistEntryIsNarrow(entry);
  }

  const state: GuardState = {
    allowlist,
    violations: [],
    originalWarn: console.warn.bind(console),
    originalError: console.error.bind(console),
  };

  console.warn = (...args: unknown[]) => {
    recordConsoleCall("warn", args, state);
    state.originalWarn(...args);
  };
  console.error = (...args: unknown[]) => {
    recordConsoleCall("error", args, state);
    state.originalError(...args);
  };

  activeGuard = state;

  return () => {
    console.warn = state.originalWarn;
    console.error = state.originalError;
    activeGuard = undefined;
  };
}

/** Throws when the active guard recorded unallowlisted console output. */
export function assertStrictConsoleClean(): void {
  const state = activeGuard;
  if (!state) {
    throw new Error(
      "Strict console guard is not installed; call installStrictConsoleGuard first.",
    );
  }

  if (state.violations.length === 0) {
    return;
  }

  const details = formatViolations(state.violations);
  state.violations.length = 0;
  throw new Error(`Unexpected console output in guarded test:\n${details}`);
}

/** Opt-in Vitest hook pair for a describe block or file-local suite. */
export function useStrictConsoleGuard(
  options: StrictConsoleGuardOptions = {},
): void {
  let dispose: (() => void) | undefined;

  beforeEach(() => {
    dispose = installStrictConsoleGuard(options);
  });

  afterEach(() => {
    try {
      assertStrictConsoleClean();
    } finally {
      dispose?.();
      dispose = undefined;
    }
  });
}

/** Runs a callback under a temporary strict console guard. */
export async function withStrictConsole<T>(
  options: StrictConsoleGuardOptions,
  callback: () => T | Promise<T>,
): Promise<T> {
  const dispose = installStrictConsoleGuard(options);
  try {
    const result = await callback();
    assertStrictConsoleClean();
    return result;
  } finally {
    dispose();
  }
}
