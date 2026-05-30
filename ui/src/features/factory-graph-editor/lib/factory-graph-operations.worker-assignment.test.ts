import { describe, expect, it } from "vitest";

import {
  addFactoryGraphNode,
  buildFactoryGraphSaveInput,
  connectFactoryGraphNodes,
  createEmptyFactoryGraphDraft,
  disconnectFactoryGraphEdge,
} from "../public";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";

describe("factory graph operations worker assignment", () => {
  it("accepts worker-assignment replacement while the draft is temporarily save-invalid", () => {
    const withReviewer = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      node: {
        kind: "worker",
        model: "gpt-5-mini",
        name: "reviewer",
      },
    });
    expect(withReviewer.ok).toBe(true);

    const disconnected = disconnectFactoryGraphEdge({
      baseFactoryDefinition,
      draft: withReviewer.ok ? withReviewer.value : createEmptyFactoryGraphDraft(),
      edgeId: "worker-assignment:worker:writer->workstation:draft",
    });
    expect(disconnected).toMatchObject({ ok: true });
    if (!disconnected.ok) {
      return;
    }

    const saveAfterDisconnect = buildFactoryGraphSaveInput({
      baseFactoryDefinition,
      draft: disconnected.value,
    });
    expect(saveAfterDisconnect).toMatchObject({
      ok: false,
      reason: "INVALID_SAVE",
      validationErrors: [
        expect.objectContaining({
          code: "MISSING_REQUIRED_FIELD",
          field: "worker",
        }),
      ],
    });

    const connected = connectFactoryGraphNodes({
      baseFactoryDefinition,
      draft: disconnected.value,
      sourceAnchorId: "worker-assignment-source",
      sourceNodeId: "worker:reviewer",
      targetAnchorId: "worker-assignment-target",
      targetNodeId: "workstation:draft",
    });
    expect(connected).toMatchObject({ ok: true });
    if (!connected.ok) {
      return;
    }

    expect(connected.value.edgeChanges).toMatchObject({
      removals: [
        {
          kind: "worker-assignment",
          source: {
            kind: "worker",
            name: "writer",
          },
          target: {
            kind: "workstation",
            name: "draft",
          },
        },
      ],
      additions: [
        {
          kind: "worker-assignment",
          source: {
            kind: "worker",
            name: "reviewer",
          },
          target: {
            kind: "workstation",
            name: "draft",
          },
        },
      ],
    });
  });
});
