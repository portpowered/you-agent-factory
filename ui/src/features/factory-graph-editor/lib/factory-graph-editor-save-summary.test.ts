import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import { buildFactoryGraphSaveSummary } from "./factory-graph-editor-save-summary";

describe("buildFactoryGraphSaveSummary", () => {
  it("summarizes created, deleted, and changed graph items for topology-only saves", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.workers.push({
      model: "gpt-5-mini",
      name: "writer",
      type: "MODEL_WORKER",
    });
    draft.additions.workstations.push({
      body: "Draft work.",
      inputs: [],
      name: "review",
      outputs: [],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    });
    draft.removals.resources.push("gpu");
    draft.edgeChanges.additions.push({
      kind: "worker-assignment",
      source: {
        kind: "worker",
        name: "writer",
      },
      target: {
        kind: "workstation",
        name: "review",
      },
    });

    expect(
      buildFactoryGraphSaveSummary({
        draft,
        hasTopologyChanges: true,
      }),
    ).toEqual({
      changedEdges: 1,
      confirmActionLabel: "Save topology",
      createdEntities: 2,
      description:
        "This save will apply 2 created entities, 1 deleted entity and 1 changed edge.",
      dirtyState: {
        layoutDirty: false,
        preferencesDirty: false,
        topologyDirty: true,
      },
      kind: "topology-only",
      removedEntities: 1,
    });
  });

  it("returns a layout-only summary when only shared layout changed", () => {
    expect(
      buildFactoryGraphSaveSummary({
        draft: createEmptyFactoryGraphDraft(),
        hasLayoutChanges: true,
      }),
    ).toEqual({
      changedEdges: 0,
      confirmActionLabel: "Save layout",
      createdEntities: 0,
      description:
        "This save will update shared graph layout positions and viewport. Factory topology stays unchanged.",
      dirtyState: {
        layoutDirty: true,
        preferencesDirty: false,
        topologyDirty: false,
      },
      kind: "layout-only",
      removedEntities: 0,
    });
  });

  it("returns a mixed summary when layout and topology both changed", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.workers.push({
      model: "gpt-5-mini",
      name: "writer",
      type: "MODEL_WORKER",
    });

    expect(
      buildFactoryGraphSaveSummary({
        draft,
        hasLayoutChanges: true,
        hasTopologyChanges: true,
      }).description,
    ).toBe(
      "This save will update shared graph layout and apply topology changes: 1 created entity.",
    );
  });

  it("returns a preferences-only summary without portable document changes", () => {
    expect(
      buildFactoryGraphSaveSummary({
        draft: createEmptyFactoryGraphDraft(),
        hasPreferenceChanges: true,
      }),
    ).toEqual({
      changedEdges: 0,
      confirmActionLabel: "Save preferences",
      createdEntities: 0,
      description:
        "Visibility and filter preferences changed for your view only. They stay private and are not saved into the shared factory document.",
      dirtyState: {
        layoutDirty: false,
        preferencesDirty: true,
        topologyDirty: false,
      },
      kind: "preferences-only",
      removedEntities: 0,
    });
  });

  it("returns an empty summary when no pending changes exist", () => {
    expect(
      buildFactoryGraphSaveSummary(createEmptyFactoryGraphDraft()),
    ).toEqual({
      changedEdges: 0,
      confirmActionLabel: "Save changes",
      createdEntities: 0,
      description: "No shared factory document changes are pending.",
      dirtyState: {
        layoutDirty: false,
        preferencesDirty: false,
        topologyDirty: false,
      },
      kind: "none",
      removedEntities: 0,
    });
  });

  it("summarizes graph edits in a non-default locale", () => {
    const draft = createEmptyFactoryGraphDraft();
    draft.additions.workers.push({
      model: "gpt-5-mini",
      name: "writer",
      type: "MODEL_WORKER",
    });
    draft.edgeChanges.removals.push({
      kind: "worker-assignment",
      source: {
        kind: "worker",
        name: "writer",
      },
      target: {
        kind: "workstation",
        name: "review",
      },
    });

    expect(
      buildFactoryGraphSaveSummary(
        {
          draft,
          hasTopologyChanges: true,
        },
        "zh-CN",
      ).description,
    ).toBe("此保存将应用 1 个新增实体 和 1 条更改边。");
  });
});
