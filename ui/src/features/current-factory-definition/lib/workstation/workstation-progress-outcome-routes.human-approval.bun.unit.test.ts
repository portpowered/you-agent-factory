import { describe, expect, it } from "bun:test";
import {
  workstationHasZAxisIncompleteForConnections,
  workstationSupportsProgressOutcomeFailureRoute,
  workstationSupportsProgressOutcomeRoutes,
} from "../workstation-progress-outcome-routes";

describe("human approval progress outcome routes", () => {
  it("does not expose progress, failure, or z-axis routes", () => {
    const workstation = {
      behavior: "STANDARD" as const,
      type: "HUMAN_APPROVAL" as const,
    };

    expect(workstationSupportsProgressOutcomeRoutes(workstation)).toBe(false);
    expect(workstationSupportsProgressOutcomeFailureRoute(workstation)).toBe(
      false,
    );
    expect(workstationHasZAxisIncompleteForConnections(workstation)).toBe(
      false,
    );
  });
});
