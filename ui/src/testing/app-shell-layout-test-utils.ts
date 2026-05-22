import type { DashboardSnapshot } from "../api/dashboard";
import type { FactoryEvent } from "../api/events";
import { FACTORY_EVENT_TYPES } from "../api/events";
import {
  buildDashboardSnapshotFixture,
  oneNodeDashboardTopology,
} from "../components/dashboard/fixtures";
import { twentyNodeDashboardSnapshot } from "../components/dashboard/test-fixtures";

export const tickZeroInitialStructureRequestEvents: FactoryEvent[] = [
  {
    context: {
      eventTime: "2026-04-16T12:00:00Z",
      sequence: 0,
      tick: 0,
    },
    id: "timeline-zero-1",
    payload: {
      factory: {
        workTypes: [
          {
            name: "story",
            states: [
              { name: "new", type: "INITIAL" },
              { name: "review", type: "PROCESSING" },
            ],
          },
        ],
        workstations: [
          {
            id: "review",
            inputs: [{ state: "new", workType: "story" }],
            name: "Review",
            outputs: [{ state: "review", workType: "story" }],
            worker: "reviewer",
          },
        ],
      },
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  },
];

export const singleNodeSnapshotWithoutEdges = {
  ...buildDashboardSnapshotFixture(oneNodeDashboardTopology),
  topology: (({ edges: _edges, ...topology }) => topology)(
    oneNodeDashboardTopology,
  ),
} as unknown as DashboardSnapshot;

export const twentyNodeSnapshot = twentyNodeDashboardSnapshot;
