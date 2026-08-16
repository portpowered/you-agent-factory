import { createMaterializedWorkOutcomeState } from "../../../work-outcome/public/materializer";
import {
  type DashboardWorkOutcomeStream,
  selectDashboardWorkOutcomeInput,
} from "../use-dashboard-bento-snapshot";

const resolvedStream: DashboardWorkOutcomeStream = {
  identity: {
    backendScopeID: "backend-a",
    factorySessionID: "session-a",
    logicalSessionKeyID: "logical-a",
    streamGenerationID: "generation-b",
  },
  status: "ready",
};

describe("dashboard work outcome stream selection", () => {
  it("keeps stale active outcomes hidden until the resolved entry is published", () => {
    const staleState = createMaterializedWorkOutcomeState();

    expect(
      selectDashboardWorkOutcomeInput(
        {
          entriesByKey: {},
          materializedWorkOutcomeState: staleState,
          selectedTick: 99,
        },
        resolvedStream,
      ),
    ).toEqual({
      hydrationStatus: "loading",
      materializedWorkOutcomeState: undefined,
      selectedTimelineTick: 0,
    });
  });

  it("selects materialized outcomes from the exact resolved generation", () => {
    const staleState = createMaterializedWorkOutcomeState();
    const resolvedState = {
      ...createMaterializedWorkOutcomeState(),
      counts: {
        completed: 4,
        dispatched: 4,
        failed: 0,
        inFlight: 0,
        queued: 0,
      },
    };

    expect(
      selectDashboardWorkOutcomeInput(
        {
          entriesByKey: {
            [JSON.stringify(["backend-a", "session-a", "generation-b"])]: {
              materializedWorkOutcomeState: resolvedState,
              selectedTick: 42,
            },
          },
          materializedWorkOutcomeState: staleState,
          selectedTick: 99,
        },
        resolvedStream,
      ),
    ).toEqual({
      hydrationStatus: "ready",
      materializedWorkOutcomeState: resolvedState,
      selectedTimelineTick: 42,
    });
  });
});
