import { describe, expect, test, vi } from "vitest";

import {
  PUBLIC_PACKAGE_RELEASE_STEPS,
  verifyPublicPackageRelease,
} from "./verify-public-package-release.mjs";

const packageStepLabels = PUBLIC_PACKAGE_RELEASE_STEPS.map(
  ({ packageName, stepName }) => `${packageName}: ${stepName}`,
);

describe("verifyPublicPackageRelease", () => {
  test("installs and verifies all packages in dependency-safe order", async () => {
    const observed = [];

    await verifyPublicPackageRelease({
      log: vi.fn(),
      runStep: vi.fn(async ({ packageName, stepName }) => {
        observed.push(`${packageName}: ${stepName}`);
      }),
    });

    expect(observed).toEqual(packageStepLabels);
    expect(observed).toEqual([
      "Public package family: install locked UI prerequisites",
      "Public package family: run orchestration regression tests",
      "@you-agent-factory/client: run release gate",
      "@you-agent-factory/components: run release gate",
      "@you-agent-factory/factory-replay: run release gate",
      "@you-agent-factory/factory-replay consumer: run 10,000-event retained-memory regression",
      "Public package family: link built workspace dependencies",
      "@you-agent-factory/factory-graph: run release gate",
      "@you-agent-factory/factory-emulator: run release gate",
      "@you-agent-factory/factory-visualizers: run release gate",
      "Website Factory emulator adapter and customer demos: run focused state and component regressions",
      "Website Factory emulator adapter and customer demos: build browser acceptance stories",
      "Website Factory emulator adapter and customer demos: run desktop, narrow, and reduced-motion browser checks",
    ]);
  });

  test.each([
    ["install locked UI prerequisites", 0],
    ["run release gate", 4],
    ["run 10,000-event retained-memory regression", 5],
    ["run focused state and component regressions", 10],
    ["build browser acceptance stories", 11],
    ["run desktop, narrow, and reduced-motion browser checks", 12],
  ])(
    "stops after the first failed %s and reports its command outcome",
    async (_failureKind, failedStepIndex) => {
      const started = [];
      const failedStep = PUBLIC_PACKAGE_RELEASE_STEPS[failedStepIndex];
      const runStep = vi.fn(async (step) => {
        started.push(`${step.packageName}: ${step.stepName}`);
        if (step === failedStep) {
          throw new Error("simulated exit code 23");
        }
      });

      await expect(
        verifyPublicPackageRelease({ log: vi.fn(), runStep }),
      ).rejects.toThrow(
        `Public package release failed at ${failedStep.packageName}: ${failedStep.stepName} (bun ${failedStep.args.join(" ")}): simulated exit code 23`,
      );

      expect(started).toEqual(packageStepLabels.slice(0, failedStepIndex + 1));
      expect(started).not.toContain(packageStepLabels[failedStepIndex + 1]);
    },
  );
});
