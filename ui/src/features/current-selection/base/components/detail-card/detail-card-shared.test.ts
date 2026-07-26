import { describe, expect, it } from "vitest";

import {
  emptyStatePlaceMessage,
  isTerminalOrFailedPlace,
  normalizeDetailText,
} from "./detail-card-shared";

describe("detail-card-shared helpers", () => {
  it("recognizes terminal and failed places", () => {
    expect(
      isTerminalOrFailedPlace({
        place_id: "done",
        state_category: "TERMINAL",
      }),
    ).toBe(true);
    expect(
      isTerminalOrFailedPlace({
        place_id: "failed",
        state_category: "FAILED",
      }),
    ).toBe(true);
    expect(
      isTerminalOrFailedPlace({
        place_id: "active",
        state_category: "ACTIVE",
      }),
    ).toBe(false);
  });

  it("selects the empty-state copy for retained and non-retained work", () => {
    const messages = {
      noCurrentWorkInPlace: "No current work.",
      noWorkRecordedAtSelectedTick: "No work recorded.",
      selectedTickWorkUnavailable: "Work unavailable.",
    };

    expect(emptyStatePlaceMessage(messages, false, 1)).toBe("No current work.");
    expect(emptyStatePlaceMessage(messages, true, 1)).toBe("Work unavailable.");
    expect(emptyStatePlaceMessage(messages, true, 0)).toBe("No work recorded.");
  });

  it("normalizes blank detail text to undefined", () => {
    expect(normalizeDetailText("  details  ")).toBe("details");
    expect(normalizeDetailText("   ")).toBeUndefined();
    expect(normalizeDetailText(undefined)).toBeUndefined();
  });
});
