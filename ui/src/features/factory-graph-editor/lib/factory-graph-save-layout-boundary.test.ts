import { describe, expect, it } from "vitest";

import { applyFactoryGraphAddEntityDraft } from "./factory-graph-editor-additions";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import {
  addFactoryGraphNode,
  applyFactoryGraphPendingEdits,
  buildFactoryGraphState,
} from "./factory-graph-operations";
import {
  factoryDefinitionSavePayloadHasGraphLayoutFields,
  findGraphLayoutPropertyPaths,
} from "./factory-graph-save-layout-boundary";

describe("findGraphLayoutPropertyPaths", () => {
  it("detects nested layout coordinate metadata", () => {
    expect(
      findGraphLayoutPropertyPaths({
        workstations: [
          {
            layout: { x: 10, y: 20 },
            name: "draft",
          },
        ],
      }),
    ).toEqual(
      expect.arrayContaining([
        "workstations[0].layout",
        "workstations[0].layout.x",
      ]),
    );
  });

  it("ignores unrelated factory definition fields", () => {
    expect(
      findGraphLayoutPropertyPaths({
        name: "Current Factory",
        workstations: [
          {
            body: "Draft the story.",
            name: "draft",
            worker: "writer",
          },
        ],
      }),
    ).toEqual([]);
  });
});

describe("factory graph save layout boundary", () => {
  it("does not add layout fields when adding a workstation to the draft", () => {
    const addResult = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      node: {
        behavior: "DEFAULT",
        body: "Review the draft.",
        kind: "workstation",
        name: "review-station",
        workerName: "writer",
      },
    });
    expect(addResult.ok).toBe(true);
    if (!addResult.ok) {
      return;
    }

    expect(findGraphLayoutPropertyPaths(addResult.value)).toEqual([]);
  });

  it("does not add layout fields to the save payload for a newly added workstation", () => {
    const addResult = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      node: {
        behavior: "DEFAULT",
        body: "Review the draft.",
        kind: "workstation",
        name: "review-station",
        workerName: "writer",
      },
    });
    expect(addResult.ok).toBe(true);
    if (!addResult.ok) {
      return;
    }

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: addResult.value,
    });
    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(
      factoryDefinitionSavePayloadHasGraphLayoutFields(saveInput.value),
    ).toBe(false);
    expect(
      saveInput.value.workstations?.find(
        (workstation) => workstation.name === "review-station",
      ),
    ).toEqual(
      expect.objectContaining({
        body: "Review the draft.",
        name: "review-station",
        worker: "writer",
      }),
    );
  });

  it("keeps graph draft additions free of layout metadata for all add kinds", () => {
    let draft = createEmptyFactoryGraphDraft();

    draft = applyFactoryGraphAddEntityDraft(draft, {
      capacity: "2",
      kind: "resource",
      name: "gpu",
    });
    draft = applyFactoryGraphAddEntityDraft(draft, {
      argsText: "",
      command: "node",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      workerType: "SCRIPT_WORKER",
    });
    draft = applyFactoryGraphAddEntityDraft(draft, {
      initialStateName: "queued",
      kind: "work-type",
      name: "task",
    });
    draft = applyFactoryGraphAddEntityDraft(draft, {
      kind: "work-state",
      name: "done",
      stateType: "TERMINAL",
      workTypeName: "task",
    });
    draft = applyFactoryGraphAddEntityDraft(draft, {
      behavior: "DEFAULT",
      body: "",
      kind: "workstation",
      name: "process",
      workerName: "runner",
    });

    expect(findGraphLayoutPropertyPaths(draft)).toEqual([]);
    expect(
      factoryDefinitionSavePayloadHasGraphLayoutFields(
        buildFactoryGraphState({
          baseFactoryDefinition,
          draft,
        }).saveInput,
      ),
    ).toBe(false);
  });
});
