import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_NODE_FAMILIES,
  type FactoryGraphNodeFamily,
} from "./node-family.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";

function visualState(
  overrides: Partial<Parameters<typeof resolveFactoryGraphVisualState>[0]> = {},
) {
  return resolveFactoryGraphVisualState({
    family: "work-state",
    ...overrides,
  });
}

describe("resolveFactoryGraphVisualState", () => {
  it.each([
    ["INITIAL", "initial", "waiting", "waiting"],
    ["PROCESSING", "processing", "active", "processing"],
    ["ACCEPTED", "terminal", "success", "completed"],
    ["CONTINUE", "terminal", "success", "completed"],
    ["TERMINAL", "terminal", "success", "completed"],
    ["FAILED", "failed", "danger", "failed"],
  ] as const)(
    "maps lifecycle %s to the %s semantic role",
    (lifecycle, lifecycleRole, status, treatment) => {
      expect(visualState({ lifecycle })).toMatchObject({
        border: status,
        icon: status,
        lifecycle: lifecycleRole,
        status,
        statusTreatment: treatment,
        surface: status,
      });
    },
  );

  it("uses a runtime status when lifecycle input is absent", () => {
    expect(visualState({ runtimeStatus: "active" })).toMatchObject({
      lifecycle: "processing",
      status: "active",
      statusTreatment: "processing",
    });
  });

  it("keeps absent and unknown lifecycle input quiet", () => {
    expect(visualState()).toMatchObject({
      border: "quiet",
      emphasis: "quiet",
      glow: "none",
      lifecycle: "unknown",
      status: "quiet",
      statusTreatment: "none",
      surface: "quiet",
    });
    expect(visualState({ lifecycle: "future-phase" })).toMatchObject({
      lifecycle: "unknown",
      status: "quiet",
    });
  });

  it("keeps lifecycle status visible under selected active flow", () => {
    expect(
      visualState({
        activeFlow: true,
        focused: true,
        lifecycle: "PROCESSING",
        selected: true,
      }),
    ).toMatchObject({
      activeFlow: true,
      border: "selection",
      emphasis: "selected",
      glow: "selection",
      focus: "selection-and-keyboard",
      icon: "active",
      lifecycle: "processing",
      selection: true,
      status: "active",
      surface: "active",
    });
  });

  it("keeps danger lifecycle visible under validation", () => {
    expect(
      visualState({ lifecycle: "FAILED", validation: true }),
    ).toMatchObject({
      border: "validation",
      glow: "validation",
      icon: "danger",
      lifecycle: "failed",
      status: "danger",
      surface: "danger",
      validation: "error",
    });
  });

  it("keeps selection and muted state independent", () => {
    expect(
      visualState({
        lifecycle: "INITIAL",
        muted: true,
        selected: true,
      }),
    ).toMatchObject({
      border: "selection",
      focus: "selection",
      lifecycle: "initial",
      muted: true,
      status: "waiting",
      surface: "waiting",
    });
  });

  it("lets validation retain priority over active flow without losing success", () => {
    expect(
      visualState({
        activeFlow: true,
        lifecycle: "TERMINAL",
        validation: "warning",
      }),
    ).toMatchObject({
      activeFlow: true,
      border: "validation",
      emphasis: "attention",
      glow: "validation",
      lifecycle: "terminal",
      status: "success",
      surface: "success",
      validation: "warning",
    });
  });

  it.each(FACTORY_GRAPH_NODE_FAMILIES as readonly FactoryGraphNodeFamily[])(
    "preserves the %s family while resolving state",
    (family) => {
      expect(visualState({ family }).family).toBe(family);
    },
  );
});

describe("resolveFactoryGraphVisualState work occupancy", () => {
  it("keeps an empty processing work state on a soft fill", () => {
    expect(visualState({ lifecycle: "PROCESSING" })).toMatchObject({
      emphasis: "standard",
      fill: "soft",
      glow: "none",
      status: "active",
      surface: "active",
    });
  });

  it("fills a processing work state solidly once it holds work", () => {
    expect(
      visualState({ activeWork: true, lifecycle: "PROCESSING" }),
    ).toMatchObject({
      emphasis: "strong",
      fill: "solid",
      glow: "active",
      status: "active",
      surface: "active",
    });
  });

  it("keeps a held terminal work state solid in its own tone", () => {
    expect(
      visualState({ activeWork: true, lifecycle: "TERMINAL" }),
    ).toMatchObject({
      fill: "solid",
      status: "success",
      surface: "success",
    });
  });

  it("treats an active flow highlight as held work", () => {
    expect(visualState({ activeFlow: true })).toMatchObject({
      fill: "solid",
      surface: "active",
    });
  });

  it("leaves a toneless node soft even while it holds work", () => {
    expect(visualState({ activeWork: true })).toMatchObject({
      fill: "soft",
      status: "quiet",
      surface: "quiet",
    });
  });

  it.each([
    ["INITIAL", "waiting"],
    ["TERMINAL", "success"],
    ["FAILED", "danger"],
  ] as const)("keeps an empty %s work state soft", (lifecycle, status) => {
    expect(visualState({ lifecycle })).toMatchObject({
      fill: "soft",
      surface: status,
    });
  });
});

describe("resolveFactoryGraphVisualState active emphasis", () => {
  it("elevates an active flow without inventing a lifecycle status", () => {
    expect(visualState({ activeFlow: true })).toMatchObject({
      activeFlow: true,
      border: "active",
      emphasis: "strong",
      glow: "active",
      icon: "active",
      lifecycle: "unknown",
      status: "quiet",
      statusTreatment: "processing",
      surface: "active",
    });
  });

  it("keeps keyboard focus visible independently of selection", () => {
    expect(visualState({ focused: true })).toMatchObject({
      border: "selection",
      emphasis: "selected",
      focus: "keyboard",
      glow: "selection",
      selection: false,
    });
  });
});
