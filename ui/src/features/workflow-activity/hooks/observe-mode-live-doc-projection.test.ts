import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES } from "../../../api/events";
import {
  normalizeFactoryDefinition,
  preserveExistingBundledFilesWhenAbsent,
} from "../../../api/factory-definition";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { syncCurrentFactoryDefinition } from "../../dashboard/lib/dashboard-event-stream";
import { listFactoryBundledDocs } from "../lib/factory-bundled-docs";
import { currentActivityCardFactoryDefinition } from "./current-activity-card-factory-definition";

const sessionID = "~default";

const timelineFactory = normalizeFactoryDefinition({
  name: "factory",
  resources: [{ capacity: 10, name: "executor-slot" }],
  workTypes: [
    {
      name: "task",
      states: [
        { name: "init", type: "INITIAL" },
        { name: "complete", type: "TERMINAL" },
      ],
    },
  ],
  workers: [{ name: "planner", type: "MODEL_WORKER" }],
  workstations: [
    {
      id: "plan",
      inputs: [{ state: "init", workType: "task" }],
      name: "plan",
      outputs: [{ state: "complete", workType: "task" }],
      type: "MODEL_WORKSTATION",
      worker: "planner",
    },
  ],
});

const savedFactoryDocument = normalizeFactoryDefinition({
  ...timelineFactory,
  supportingFiles: {
    bundledFiles: [
      {
        content: { encoding: "utf-8", inline: "# Overview" },
        targetPath: "factory/docs/overview.md",
        type: "DOC",
      },
      {
        content: { encoding: "utf-8", inline: "# Planning" },
        targetPath: "factory/docs/planning.md",
        type: "DOC",
      },
    ],
  },
  version: {
    logical: "1",
    physical: "2026-06-08T00:00:00Z",
  },
});

function factoryChangeEvent(factory: typeof timelineFactory) {
  return {
    context: { eventTime: "2026-06-08T04:00:00Z", sequence: 1, tick: 1 },
    id: "event-1",
    payload: { factory },
    type: FACTORY_EVENT_TYPES.factoryChange,
  } as const;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this regression keeps the event stream, query cache, and graph projection flow in one harness.
describe("observe-mode live doc projection", () => {
  it("keeps bundled docs for graph projection after event-stream cache sync and factory GET", () => {
    const queryClient = new QueryClient();

    syncCurrentFactoryDefinition(
      queryClient,
      factoryChangeEvent(timelineFactory),
      sessionID,
    );

    queryClient.setQueryData(
      currentFactoryDocumentQueryKey(sessionID),
      savedFactoryDocument,
    );
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(sessionID),
      savedFactoryDocument,
    );

    syncCurrentFactoryDefinition(
      queryClient,
      factoryChangeEvent({
        ...timelineFactory,
        version: { logical: "2", physical: "2026-06-08T04:01:00Z" },
      }),
      sessionID,
    );

    const document = queryClient.getQueryData(
      currentFactoryDocumentQueryKey(sessionID),
    );
    const editor = {
      draftState: {},
      editableFactoryDocument: document,
      editableFactoryDocumentStatus: "success" as const,
      editorMode: false,
    };

    const displayFactory = currentActivityCardFactoryDefinition(
      editor as never,
      { factory: timelineFactory } as never,
    );

    expect(listFactoryBundledDocs(displayFactory)).toEqual([
      expect.objectContaining({ targetPath: "factory/docs/overview.md" }),
      expect.objectContaining({ targetPath: "factory/docs/planning.md" }),
    ]);
  });

  it("uses bundled docs from cached document data while the factory query is still pending", () => {
    const editor = {
      draftState: {},
      editableFactoryDocument: savedFactoryDocument,
      editableFactoryDocumentStatus: "pending" as const,
      editorMode: false,
    };

    const displayFactory = currentActivityCardFactoryDefinition(
      editor as never,
      { factory: timelineFactory } as never,
    );

    expect(listFactoryBundledDocs(displayFactory)).toEqual([
      expect.objectContaining({ targetPath: "factory/docs/overview.md" }),
      expect.objectContaining({ targetPath: "factory/docs/planning.md" }),
    ]);
  });

  it("does not invent bundled docs before the saved factory document is available", () => {
    const editor = {
      draftState: {},
      editableFactoryDocument: undefined,
      editableFactoryDocumentStatus: "pending" as const,
      editorMode: false,
    };

    const displayFactory = currentActivityCardFactoryDefinition(
      editor as never,
      { factory: timelineFactory } as never,
    );

    expect(listFactoryBundledDocs(displayFactory)).toEqual([]);
  });

  it("keeps snapshot layout for observer projection when cached document data omits layout", () => {
    const editor = {
      draftState: {},
      editableFactoryDocument: {
        ...savedFactoryDocument,
        layout: undefined,
      },
      editableFactoryDocumentStatus: "success" as const,
      editorMode: false,
    };
    const snapshotFactory = {
      ...timelineFactory,
      layout: {
        nodes: [
          {
            id: "workstation:plan",
            position: { x: 420, y: 180 },
          },
        ],
        schemaVersion: 1,
        viewport: { x: 18, y: 22, zoom: 1.15 },
      },
    };

    const displayFactory = currentActivityCardFactoryDefinition(
      editor as never,
      { factory: snapshotFactory } as never,
    );

    expect(displayFactory?.layout).toEqual(snapshotFactory.layout);
  });

  it("keeps bundled docs when FACTORY_CHANGE sync completes after the factory GET lands", () => {
    const queryClient = new QueryClient();
    const initialFactory = preserveExistingBundledFilesWhenAbsent(
      timelineFactory,
      undefined,
    );

    syncCurrentFactoryDefinition(
      queryClient,
      factoryChangeEvent(initialFactory),
      sessionID,
    );

    queryClient.setQueryData(
      currentFactoryDocumentQueryKey(sessionID),
      savedFactoryDocument,
    );

    syncCurrentFactoryDefinition(
      queryClient,
      factoryChangeEvent({
        ...timelineFactory,
        version: { logical: "2", physical: "2026-06-08T04:01:00Z" },
      }),
      sessionID,
    );

    expect(
      listFactoryBundledDocs(
        queryClient.getQueryData(currentFactoryDocumentQueryKey(sessionID)),
      ),
    ).toEqual([
      expect.objectContaining({ targetPath: "factory/docs/overview.md" }),
      expect.objectContaining({ targetPath: "factory/docs/planning.md" }),
    ]);
  });
});
