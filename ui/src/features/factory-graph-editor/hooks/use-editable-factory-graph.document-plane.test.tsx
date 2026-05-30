import { renderHook } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { useEditableFactoryGraph } from "./use-editable-factory-graph";

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
    logical: "9",
    physical: "2026-05-31T01:00:00Z",
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

describe("useEditableFactoryGraph document plane", () => {
  it("projects editable graph nodes from the loaded factory document only", () => {
    const { result } = renderHook(() =>
      useEditableFactoryGraph({
        currentFactoryDocument: documentFactory,
      }),
    );

    expect(result.current.draftState.source).toBe("current-factory");
    expect(result.current.draftState.baseDocument).toEqual(documentFactory);
    expect(result.current.draftState.latestDocument).toEqual(documentFactory);
    expect(result.current.projection.nodes.map((node) => node.id)).toContain(
      "workstation:document-only",
    );
    expect(result.current.projection.nodes.map((node) => node.id)).not.toContain(
      "workstation:snapshot-only",
    );
  });

  it("exposes an empty projection while the factory document is still unavailable", () => {
    const { result } = renderHook(() => useEditableFactoryGraph({}));

    expect(result.current.draftState.source).toBe("projection");
    expect(result.current.draftState.baseDocument).toBeNull();
    expect(result.current.draftState.latestDocument).toBeNull();
    expect(result.current.projection).toEqual({
      edges: [],
      nodes: [],
    });
    expect(result.current.graphState).toBeNull();
  });
});
