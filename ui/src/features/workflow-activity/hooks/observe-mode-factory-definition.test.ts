import { describe, expect, it } from "vitest";

import { buildDashboardSnapshotFixture } from "../../../components/dashboard/fixtures";
import { mediumBranchingDashboardTopology } from "../../../components/dashboard/fixtures/topologies";
import {
  baseFactoryDefinitionDocument,
  buildDivergentSnapshotPlaneFactory,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import { resolveObserveModeFactoryDefinition } from "./observe-mode-factory-definition";

describe("resolveObserveModeFactoryDefinition", () => {
  it("prefers the saved document when the snapshot plane is a strict workstation superset", () => {
    const snapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology);
    snapshot.factory = buildDivergentSnapshotPlaneFactory();

    expect(
      resolveObserveModeFactoryDefinition({
        document: divergentDocumentPlaneFactoryDocument,
        snapshotFactory: snapshot.factory,
        timelineMode: "current",
      }),
    ).toEqual(divergentDocumentPlaneFactoryDocument);
  });

  it("prefers the timeline snapshot factory when replay diverges structurally at the current tick", () => {
    const snapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology);
    const replayFactory = {
      ...baseFactoryDefinitionDocument,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" as const },
            { name: "done", type: "TERMINAL" as const },
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          outputs: [{ state: "done", workType: "story" }],
          type: "MODEL_WORKSTATION" as const,
          worker: "reviewer",
        },
      ],
    };
    snapshot.factory = replayFactory;

    expect(
      resolveObserveModeFactoryDefinition({
        document: baseFactoryDefinitionDocument,
        snapshotFactory: replayFactory,
        timelineMode: "current",
      }),
    ).toEqual(replayFactory);
  });

  it("prefers the timeline snapshot factory while scrubbing a fixed tick", () => {
    const snapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology);

    expect(
      resolveObserveModeFactoryDefinition({
        document: divergentDocumentPlaneFactoryDocument,
        snapshotFactory: snapshot.factory,
        timelineMode: "fixed",
      }),
    ).toEqual(snapshot.factory);
  });
});
