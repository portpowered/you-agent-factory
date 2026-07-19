import { describe, expect, it } from "vitest";

import {
  DEFAULT_FACTORY_GRAPH_CHROME_PRESET,
  resolveFactoryGraphChrome,
} from "./factory-graph-chrome";

describe("resolveFactoryGraphChrome", () => {
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
    expect(resolveFactoryGraphChrome({ preset })).toEqual(expected);
  });

  it("uses full as the established default preset", () => {
    expect(DEFAULT_FACTORY_GRAPH_CHROME_PRESET).toBe("full");
    expect(resolveFactoryGraphChrome()).toEqual(
      resolveFactoryGraphChrome({ preset: "full" }),
    );
  });

  it("applies every supplied override after resolving the preset", () => {
    expect(
      resolveFactoryGraphChrome({
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
      resolveFactoryGraphChrome({
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

    expect(resolveFactoryGraphChrome(configuration)).toEqual({
      background: true,
      legend: false,
      viewportControls: true,
      visibilityControls: true,
    });
    expect(configuration).toEqual({ legend: false, preset: "full" });
  });
});
