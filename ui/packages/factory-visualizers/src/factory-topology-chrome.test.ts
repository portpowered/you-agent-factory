import { describe, expect, it } from "vitest";

import {
  DEFAULT_FACTORY_TOPOLOGY_CHROME_PRESET,
  resolveFactoryTopologyChrome,
} from "./factory-topology-chrome";

describe("resolveFactoryTopologyChrome", () => {
  it.each([
    [
      "full",
      {
        background: true,
        legend: true,
        viewportControls: true,
        visibilityControls: true,
      },
    ],
    [
      "minimal",
      {
        background: true,
        legend: false,
        viewportControls: true,
        visibilityControls: false,
      },
    ],
    [
      "none",
      {
        background: false,
        legend: false,
        viewportControls: false,
        visibilityControls: false,
      },
    ],
  ] as const)("resolves the %s preset", (preset, expected) => {
    expect(resolveFactoryTopologyChrome({ preset })).toEqual(expected);
  });

  it("uses full as the established default preset", () => {
    expect(DEFAULT_FACTORY_TOPOLOGY_CHROME_PRESET).toBe("full");
    expect(resolveFactoryTopologyChrome()).toEqual(
      resolveFactoryTopologyChrome({ preset: "full" }),
    );
  });

  it("applies every supplied override after resolving the preset", () => {
    expect(
      resolveFactoryTopologyChrome({
        background: false,
        legend: true,
        preset: "minimal",
        visibilityControls: true,
      }),
    ).toEqual({
      background: false,
      legend: true,
      viewportControls: true,
      visibilityControls: true,
    });
    expect(
      resolveFactoryTopologyChrome({
        legend: true,
        preset: "none",
        viewportControls: true,
      }),
    ).toEqual({
      background: false,
      legend: true,
      viewportControls: true,
      visibilityControls: false,
    });
  });

  it("does not mutate caller-provided configuration", () => {
    const configuration = Object.freeze({
      legend: false,
      preset: "full" as const,
    });
    expect(resolveFactoryTopologyChrome(configuration)).toEqual({
      background: true,
      legend: false,
      viewportControls: true,
      visibilityControls: true,
    });
    expect(configuration).toEqual({ legend: false, preset: "full" });
  });
});
