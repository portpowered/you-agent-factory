import { describe, expect, it } from "vitest";

import {
  assertStrictConsoleClean,
  installStrictConsoleGuard,
  useStrictConsoleGuard,
  withStrictConsole,
} from "./strict-console-guard";

describe("strict-console-guard core behavior", () => {
  it("fails when an unexpected console.error is emitted", () => {
    const dispose = installStrictConsoleGuard();
    try {
      console.error("unexpected react act warning from LineImpl");

      let failureMessage = "";
      try {
        assertStrictConsoleClean();
      } catch (error) {
        failureMessage = error instanceof Error ? error.message : String(error);
      }

      expect(failureMessage).toMatch(/Unexpected console output/);
      expect(failureMessage).toMatch(/console\.error/);
      expect(failureMessage).toMatch(/LineImpl/);
    } finally {
      dispose();
    }
  });

  it("passes when console.error matches a named allowlist entry", async () => {
    await withStrictConsole(
      {
        allowlist: [
          {
            name: "recharts-act-line-impl",
            level: "error",
            match: "LineImpl",
            reason:
              "Recharts responsive layout still schedules one act warning in export PNG coverage.",
          },
        ],
      },
      () => {
        console.error(
          "An update to LineImpl inside a test was not wrapped in act(...).",
        );
      },
    );
  });

  it("matches allowlisted console output via RegExp entries", async () => {
    await withStrictConsole(
      {
        allowlist: [
          {
            name: "tick-slider-act",
            level: "error",
            match: /TickSliderControl inside a test/,
            reason:
              "Timeline slider async updates can flush after renderApp in replay smoke tests.",
          },
        ],
      },
      () => {
        console.error(
          "An update to TickSliderControl inside a test was not wrapped in act(...).",
        );
      },
    );
  });

  it("formats non-string console arguments when recording violations", () => {
    const dispose = installStrictConsoleGuard();
    try {
      console.error(new Error("wrapped act warning"));
      console.warn({ code: "WARN_ACT", detail: "AgentBentoLayout" });

      let failureMessage = "";
      try {
        assertStrictConsoleClean();
      } catch (error) {
        failureMessage = error instanceof Error ? error.message : String(error);
      }

      expect(failureMessage).toMatch(/wrapped act warning/);
      expect(failureMessage).toMatch(/console\.warn/);
      expect(failureMessage).toMatch(/AgentBentoLayout/);
    } finally {
      dispose();
    }
  });
});

describe("strict-console-guard allowlist validation", () => {
  it("rejects broad wildcard allowlist patterns at install time", () => {
    expect(() =>
      installStrictConsoleGuard({
        allowlist: [
          {
            name: "too-broad",
            level: "warn",
            match: /.*/,
            reason: "Should not be accepted.",
          },
        ],
      }),
    ).toThrow(/broad wildcard/);
  });

  it("rejects invalid allowlist metadata at install time", () => {
    expect(() =>
      installStrictConsoleGuard({
        allowlist: [
          {
            name: " ",
            level: "warn",
            match: "valid-match",
            reason: "Missing name.",
          },
        ],
      }),
    ).toThrow(/non-empty name/);

    expect(() =>
      installStrictConsoleGuard({
        allowlist: [
          {
            name: "missing-reason",
            level: "warn",
            match: "valid-match",
            reason: " ",
          },
        ],
      }),
    ).toThrow(/requires a reason/);

    expect(() =>
      installStrictConsoleGuard({
        allowlist: [
          {
            name: "too-short",
            level: "warn",
            match: "ab",
            reason: "Substring match must be specific.",
          },
        ],
      }),
    ).toThrow(/at least 3 characters/);
  });

  it("rejects installing a second guard in the same worker", () => {
    const dispose = installStrictConsoleGuard();
    try {
      expect(() => installStrictConsoleGuard()).toThrow(/already installed/);
    } finally {
      dispose();
    }
  });

  it("requires an installed guard before asserting cleanliness", () => {
    expect(() => assertStrictConsoleClean()).toThrow(/not installed/);
  });
});

describe("strict-console-guard useStrictConsoleGuard", () => {
  useStrictConsoleGuard();

  it("passes when the guarded test emits no console noise", () => {
    expect(true).toBe(true);
  });
});

describe("strict-console-guard allowlisted warn", () => {
  useStrictConsoleGuard({
    allowlist: [
      {
        name: "radix-dialog-description",
        level: "warn",
        match: "Missing `Description`",
        reason:
          "Radix dialog description warning is exercised in a dedicated accessibility test shim.",
      },
    ],
  });

  it("passes when console.warn matches the allowlist", () => {
    console.warn(
      'Warning: Missing `Description` or `aria-describedby={undefined}` for {DialogContent}.',
    );
  });
});
