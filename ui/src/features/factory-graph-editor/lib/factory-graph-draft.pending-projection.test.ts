import { expect, it } from "vitest";

import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import {
  applyFactoryGraphPendingEdits,
  buildFactoryGraphState,
} from "./factory-graph-operations";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";

it("projects draft-applied topology while save validation blocks missing worker assignments", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.edgeChanges.removals.push({
    kind: "worker-assignment",
    source: {
      kind: "worker",
      name: "writer",
    },
    target: {
      kind: "workstation",
      name: "draft",
    },
  });

  const state = buildFactoryGraphState({
    baseFactoryDefinition,
    draft,
  });

  expect(state.validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
      }),
    ]),
  );
  expect(state.pendingFactoryDefinition?.workstations).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        name: "draft",
        worker: "",
      }),
    ]),
  );
  expect(state.saveInput).toBeNull();
  expect(state.graph.edges.map((edge) => edge.id)).not.toContain(
    "worker-assignment:worker:writer->workstation:draft",
  );

  const saveInput = applyFactoryGraphPendingEdits({
    baseFactoryDefinition,
    draft,
  });
  const applied = applyFactoryGraphPendingEdits({
    baseFactoryDefinition,
    draft,
  });

  expect(saveInput).toMatchObject({
    ok: false,
    reason: "INVALID_SAVE",
  });
  expect(applied).toMatchObject({
    ok: false,
    reason: "INVALID_SAVE",
  });
});
