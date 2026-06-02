import { act } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  renderEditableFactoryGraphHook,
  setupEditableFactoryGraphSaveTestEnvironment,
} from "../../../testing/editable-factory-graph-hook-test-helpers";
import { mockFactoryDocumentSave } from "../../../testing/factory-document-save-mocks";

const sharedWorkType = {
  name: "story",
  states: [
    {
      name: "queued",
      type: "INITIAL" as const,
    },
    {
      name: "done",
      type: "TERMINAL" as const,
    },
  ],
};

const documentFactory: CurrentFactoryDocument = {
  name: "Document Factory",
  version: {
    logical: "11",
    physical: "2026-05-31T12:00:00Z",
  },
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [sharedWorkType],
  workstations: [
    {
      body: "Document plane baseline.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "document-only",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

/** Fixture-only: represents snapshot topology that must not become the save payload. */
const snapshotOnlyWorkstationName = "snapshot-only";

describe("useEditableFactoryGraph snapshot overlay", () => {
  beforeEach(() => {
    setupEditableFactoryGraphSaveTestEnvironment();
  });

  it("blocks save while snapshot runtime reports in-flight dispatches", () => {
    const saveMutation = setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({ mode: "success" }),
    );

    const { result } = renderEditableFactoryGraphHook({
      activeWorkCount: 2,
      currentFactoryDocument: documentFactory,
    });

    act(() => {
      result.current.actions.addNode({
        kind: "resource",
        capacity: 1,
        name: "review-slot",
      });
    });

    expect(result.current.pendingState.hasChanges).toBe(true);
    expect(result.current.saveState.canSave).toBe(false);

    act(() => {
      void result.current.actions.save();
    });

    expect(saveMutation.saveAsync).not.toHaveBeenCalled();
    expect(snapshotOnlyWorkstationName).toBe("snapshot-only");
  });

  it("builds save payloads from the factory document when snapshot factory would diverge", async () => {
    const saveMutation = setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({ mode: "success" }),
    );

    const { result } = renderEditableFactoryGraphHook({
      activeWorkCount: 0,
      currentFactoryDocument: documentFactory,
    });

    act(() => {
      result.current.actions.addNode({
        kind: "resource",
        capacity: 1,
        name: "review-slot",
      });
    });

    let didSave = false;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(true);
    expect(saveMutation.saveAsync).toHaveBeenCalledWith({
      baseVersion: documentFactory.version,
      factory: expect.objectContaining({
        workstations: expect.arrayContaining([
          expect.objectContaining({ name: "document-only" }),
        ]),
      }),
    });
    expect(saveMutation.saveAsync).toHaveBeenCalledWith({
      baseVersion: documentFactory.version,
      factory: expect.not.objectContaining({
        workstations: expect.arrayContaining([
          expect.objectContaining({ name: snapshotOnlyWorkstationName }),
        ]),
      }),
    });
  });
});
