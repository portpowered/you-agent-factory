import { describe, expect, it } from "vitest";

import { formatDashboardPlaceLabel, getDashboardPlaceLabelParts } from "./place-labels";

describe("formatDashboardPlaceLabel", () => {
  it("renders stateful place labels as work-type and state pairs", () => {
    expect(
      formatDashboardPlaceLabel({
        place_id: "story:implemented",
        state_value: "implemented",
        type_id: "story",
      }),
    ).toBe("story:implemented");
  });

  it("falls back to the raw place id when state metadata is unavailable", () => {
    expect(
      formatDashboardPlaceLabel({
        place_id: "review-queue",
      }),
    ).toBe("review-queue");
  });
});

describe("getDashboardPlaceLabelParts", () => {
  it("splits stateful place labels into display-ready work type and state values", () => {
    expect(
      getDashboardPlaceLabelParts({
        place_id: "story:implemented",
        state_value: "implemented",
        type_id: "story",
      }),
    ).toEqual({
      rawLabel: "story:implemented",
      stateValue: "implemented",
      workType: "story",
    });
  });

  it("uses safe fallback parts when a place lacks shared metadata", () => {
    expect(
      getDashboardPlaceLabelParts({
        place_id: "review-queue",
      }),
    ).toEqual({
      rawLabel: "review-queue",
      stateValue: "review-queue",
      workType: "work",
    });
  });
});

