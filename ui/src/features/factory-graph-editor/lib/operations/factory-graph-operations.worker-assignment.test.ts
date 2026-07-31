import { describe, expect, it } from "vitest";
import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "../draft/factory-graph-draft-types";
import {
  addFactoryGraphNode,
  applyFactoryGraphPendingEdits,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
} from "../operations/factory-graph-operations";

describe("factory graph operations worker assignment", () => {
  it("accepts worker-assignment replacement while the draft is temporarily save-invalid", () => {
    const withReviewer = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      node: {
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5-mini",
        modelProvider: "CODEX",
        name: "reviewer",
        operations: [
          {
            name: "REVIEW",
            inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
            outputs: [{ name: "result", contentTypes: ["TEXT"] }],
          },
        ],
        workerType: "MODEL_WORKER",
      },
    });
    expect(withReviewer.ok).toBe(true);

    const disconnected = disconnectFactoryGraphEdge({
      baseFactoryDefinition,
      draft: withReviewer.ok
        ? withReviewer.value
        : createEmptyFactoryGraphDraft(),
      edgeId: "worker-assignment:worker:writer->workstation:draft",
    });
    expect(disconnected).toMatchObject({ ok: true });
    if (!disconnected.ok) {
      return;
    }

    const saveAfterDisconnect = applyFactoryGraphPendingEdits({
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
