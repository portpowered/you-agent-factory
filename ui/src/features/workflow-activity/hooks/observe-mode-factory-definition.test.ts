import { describe, expect, it } from "vitest";

import { buildDashboardSnapshotFixture } from "../../../components/dashboard/fixtures";
import { mediumBranchingDashboardTopology } from "../../../components/dashboard/fixtures/topologies";
import {
  baseFactoryDefinitionDocument,
  buildDivergentSnapshotPlaneFactory,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import { sessionFactoryDocumentFromSnapshot } from "../../../testing/session-factory-mocks";
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

  it("prefers the saved document when it adds work type states ahead of the snapshot", () => {
    const snapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology);
    const savedDocument = {
      ...baseFactoryDefinitionDocument,
      workTypes: [
        ...(baseFactoryDefinitionDocument.workTypes ?? []),
        {
          name: "task",
          states: [{ name: "open", type: "INITIAL" as const }],
        },
      ],
    };

    expect(
      resolveObserveModeFactoryDefinition({
        document: savedDocument,
        snapshotFactory: snapshot.factory,
        timelineMode: "current",
      }),
    ).toEqual(savedDocument);
  });

  it("prefers the timeline snapshot factory when replay diverges structurally at the current tick", () => {
    const snapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology);
    const persistedDocument = sessionFactoryDocumentFromSnapshot(snapshot);
    const replayFactory = {
      ...persistedDocument,
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
        document: persistedDocument,
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

  it("prefers the timeline snapshot factory when the saved document is a minimal replay stub", () => {
    const snapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology);
    const replayStubDocument = {
      name: "Browser Replay Factory",
      version: {
        logical: "1",
        physical: "2026-06-01T00:00:00Z",
      },
    };

    expect(
      resolveObserveModeFactoryDefinition({
        document: replayStubDocument,
        snapshotFactory: snapshot.factory,
        timelineMode: "current",
      }),
    ).toEqual(snapshot.factory);
  });
});
