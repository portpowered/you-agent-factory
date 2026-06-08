import { describe, expect, it } from "vitest";
import { getCurrentSelectionOperationalEnumMessages } from "./current-selection-operational-enums";

describe("getCurrentSelectionOperationalEnumMessages", () => {
  const messages = getCurrentSelectionOperationalEnumMessages("en");

  describe("localizeWorkstationRunOutcome", () => {
    it("labels repeater rejected outcomes as repeated work for legacy lowercase kinds", () => {
      expect(
        messages.localizeWorkstationRunOutcome("REJECTED", "repeater"),
      ).toEqual({
        label: "Repeated work",
        rawOutcomeLabel: "Raw outcome: REJECTED",
      });
    });

    it("labels repeater rejected outcomes as repeated work for canonical REPEATER kinds", () => {
      expect(
        messages.localizeWorkstationRunOutcome("REJECTED", "REPEATER"),
      ).toEqual({
        label: "Repeated work",
        rawOutcomeLabel: "Raw outcome: REJECTED",
      });
    });

    it("keeps standard workstation rejected outcomes labeled as rejected", () => {
      expect(
        messages.localizeWorkstationRunOutcome("REJECTED", "standard"),
      ).toEqual({
        label: "Rejected",
      });
    });

    it("keeps cron workstation rejected outcomes labeled as rejected", () => {
      expect(
        messages.localizeWorkstationRunOutcome("REJECTED", "CRON"),
      ).toEqual({
        label: "Rejected",
      });
    });
  });
});
