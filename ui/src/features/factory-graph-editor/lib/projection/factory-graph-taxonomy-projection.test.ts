import { describe, expect, it } from "vitest";

import { WorkerType, WorkstationType } from "../../../../api/generated/openapi";
import {
  applyEditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "../draft/factory-graph-draft-types";
import { projectFactoryGraphToReactFlow } from "./factory-graph-react-flow-projection";

const legacyFactory: CanonicalFactoryDefinition = {
  name: "Legacy Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      body: "Draft the story.",
      inputs: [{ state: "queued", workType: "story" }],
      name: "draft",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

const taxonomyFactory: CanonicalFactoryDefinition = {
  ...legacyFactory,
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: WorkerType.WorkerTypeInferenceWorker,
    },
  ],
  workstations: [
    {
      ...legacyFactory.workstations![0],
      type: WorkstationType.WorkstationTypeAgentRun,
    },
  ],
};

function projectNodeIds(factory: CanonicalFactoryDefinition): string[] {
  const topology = buildFactoryGraphTopologyFromDefinition(factory);
  return projectFactoryGraphToReactFlow({ topology }).nodes.map((node) => node.id);
}

describe("factory graph taxonomy projection", () => {
  it("preserves graph node identity for legacy and new taxonomy factories", () => {
    expect(projectNodeIds(legacyFactory)).toEqual(projectNodeIds(taxonomyFactory));
    expect(projectNodeIds(legacyFactory)).toEqual([
      "worker:writer",
      "workstation:draft",
      "work-type:story",
      "work-state:story:done",
      "work-state:story:queued",
    ]);
  });

  it("round-trips new taxonomy workstation saves without downgrading to legacy names", () => {
    const values = resolveEditableWorkstationValues(taxonomyFactory, {
      node_id: "draft",
      transition_id: "draft",
      workstation_kind: WorkstationType.WorkstationTypeAgentRun,
      workstation_name: "draft",
    });
    if (!values) {
      throw new Error("expected editable workstation values");
    }

    const draft = editableWorkstationDraftFromValues({
      ...values,
      workstationType: WorkstationType.WorkstationTypeInferenceRun,
      operation: "TTS",
      operationBindings: [],
    });
    const saved = applyEditableWorkstationDraft(
      taxonomyFactory,
      {
        node_id: "draft",
        transition_id: "draft",
        workstation_kind: WorkstationType.WorkstationTypeAgentRun,
        workstation_name: "draft",
      },
      {
        ...draft,
        workstationType: WorkstationType.WorkstationTypeInferenceRun,
      },
    );

    expect(saved?.workstations?.[0]).toMatchObject({
      name: "draft",
      type: WorkstationType.WorkstationTypeInferenceRun,
      worker: "writer",
    });
  });
});
