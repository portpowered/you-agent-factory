import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS,
  factoryGraphGroupRegionColorStyle,
  normalizeFactoryGraphGroupRegionCustomColor,
  projectFactoryGraphGroupRegionBounds,
  projectFactoryGraphGroupRegions,
  resolveFactoryGraphGroupRegionColor,
} from "./group-region-presentation.js";

const group = {
  bounds: { height: 120, width: 240, x: 40, y: 60 },
  color: "info",
  id: "group-1",
  label: "Review",
};

describe("Factory graph group region presentation", () => {
  it("exposes the semantic palette and maps each supported color to safe roles", () => {
    expect(FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS).toEqual([
      "neutral",
      "primary",
      "info",
      "success",
      "warning",
      "danger",
      "outline",
    ]);

    for (const color of [
      "neutral",
      "primary",
      "info",
      "success",
      "warning",
      "danger",
    ]) {
      expect(factoryGraphGroupRegionColorStyle(color).accent).toContain(
        "var(--color-",
      );
    }
  });

  it("aliases outline and unsupported legacy values to neutral without raw CSS", () => {
    expect(resolveFactoryGraphGroupRegionColor("outline")).toBe("neutral");
    expect(resolveFactoryGraphGroupRegionColor("legacy-purple")).toBe(
      "neutral",
    );
    expect(
      factoryGraphGroupRegionColorStyle("url(javascript:alert(1))"),
    ).toEqual(factoryGraphGroupRegionColorStyle("neutral"));
  });

  it("normalizes custom colors and renders them without interpolating unsafe values", () => {
    expect(normalizeFactoryGraphGroupRegionCustomColor("#ABC123")).toBe(
      "#abc123",
    );
    expect(normalizeFactoryGraphGroupRegionCustomColor("#abc")).toBe("#aabbcc");
    expect(
      normalizeFactoryGraphGroupRegionCustomColor("url(javascript:alert(1))"),
    ).toBeNull();
    expect(factoryGraphGroupRegionColorStyle("#ABC123")).toEqual({
      accent: "#abc123",
      fill: "color-mix(in srgb, #abc123 18%, transparent)",
    });
  });

  it("projects labels and drops absent or invalid groups safely", () => {
    expect(
      projectFactoryGraphGroupRegions([
        group,
        { ...group, id: "", label: "ignored" },
        { ...group, id: "invalid", bounds: { ...group.bounds, width: 0 } },
      ]),
    ).toEqual([
      {
        bounds: group.bounds,
        color: "info",
        id: "group-1",
        label: "Review",
      },
    ]);
    expect(projectFactoryGraphGroupRegions(undefined)).toEqual([]);
    expect(
      projectFactoryGraphGroupRegions([{ ...group, label: "   " }])[0],
    ).toMatchObject({ id: "group-1", label: "group-1" });
  });

  it("keeps deterministic order for overlapping groups and scales bounds with the viewport", () => {
    const projected = projectFactoryGraphGroupRegions([
      group,
      { ...group, id: "group-2", label: "A long overlapping group label" },
    ]);

    expect(projected.map(({ id }) => id)).toEqual(["group-1", "group-2"]);
    expect(
      projectFactoryGraphGroupRegionBounds(group.bounds, [10, 20, 0.5]),
    ).toEqual({ height: 60, width: 120, x: 30, y: 50 });
  });

  it("restores a normalized custom color in the projected region", () => {
    expect(
      projectFactoryGraphGroupRegions([{ ...group, color: "#ABC123" }]),
    ).toEqual([
      {
        bounds: group.bounds,
        color: "#abc123",
        id: "group-1",
        label: "Review",
      },
    ]);
    expect(
      projectFactoryGraphGroupRegions([{ ...group, color: "not-a-color" }]),
    ).toEqual([
      {
        bounds: group.bounds,
        color: "neutral",
        id: "group-1",
        label: "Review",
      },
    ]);
  });
});
