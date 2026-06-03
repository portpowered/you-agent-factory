import { describe, expect, it } from "vitest";

import {
  assertStrictConsoleClean,
  installStrictConsoleGuard,
  useStrictConsoleGuard,
  withStrictConsole,
} from "./strict-console-guard";

describe("strict-console-guard", () => {
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

  describe("useStrictConsoleGuard", () => {
    useStrictConsoleGuard();

    it("passes when the guarded test emits no console noise", () => {
      expect(true).toBe(true);
    });
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
