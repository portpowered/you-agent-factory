import { describe, expect, it } from "vitest";

import type {
  EditableWorkstationDraft,
  EditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { resolveDraftForWorkstationTypeChange } from "./editable-workstation-type-mutators";

const baseDraft: EditableWorkstationDraft = {
  behavior: "STANDARD",
  cron: null,
  guards: [],
  inputs: [],
  name: "Review",
  operation: "",
  operationBindings: [],
  prompt: "Review prompt",
  runnerName: null,
  workerName: "reviewer",
  workstationType: "MODEL_WORKSTATION",
};

const baseValues: EditableWorkstationValues = {
  behavior: "STANDARD",
  behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
  cron: null,
  effectiveRunnerName: "codex",
  factoryRunnerName: null,
  modelInvokeWorkerOptions: ["tts-worker"],
  modelOperationsByWorkerName: {
    "tts-worker": [
      {
        name: "TTS",
        inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
        outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
      },
    ],
  },
  operation: "",
  operationBindings: [],
  prompt: "Loaded prompt",
  resolvedRunnerSelection: {
    runnerId: "codex",
    source: "default",
  },
  runnerName: null,
  runnerOptions: ["codex", "reviewer"],
  runnerSelectionSource: "default",
  sharedWorkerWorkstationNames: [],
  sharedWorkerWorkstationNamesByWorkerName: {},
  workerModelProvider: null,
  workerName: "reviewer",
  workerOptions: ["reviewer"],
  workerTypeByName: {
    reviewer: "MODEL_WORKER",
    "tts-worker": "MODEL_WORKER",
  },
  guards: [],
  inputs: [],
  workstationName: "Review",
  workstationOptions: ["Review"],
  workstationType: "MODEL_WORKSTATION",
  workstationTypeOptions: ["MODEL_WORKSTATION", "MODEL_INVOKE"],
};

describe("resolveDraftForWorkstationTypeChange", () => {
  it("returns the same draft when the workstation type is unchanged", () => {
    expect(
      resolveDraftForWorkstationTypeChange(
        baseDraft,
        "MODEL_WORKSTATION",
        baseValues,
      ),
    ).toBe(baseDraft);
  });

  it("re-seeds model-invoke fields when converting to MODEL_INVOKE", () => {
    expect(
      resolveDraftForWorkstationTypeChange(
        baseDraft,
        "MODEL_INVOKE",
        baseValues,
      ),
    ).toMatchObject({
      workstationType: "MODEL_INVOKE",
      workerName: "tts-worker",
      operation: "TTS",
      operationBindings: [
        expect.objectContaining({
          slot: "text",
        }),
      ],
    });
  });

  it("clears model-invoke fields and restores prompt when returning to MODEL_WORKSTATION", () => {
    const invokeDraft: EditableWorkstationDraft = {
      ...baseDraft,
      workstationType: "MODEL_INVOKE",
      workerName: "tts-worker",
      operation: "TTS",
      operationBindings: [
        {
          slot: "text",
          configText: "",
          defaultContentText: "",
          selector: { label: "", role: "", slot: "", type: "" },
        },
      ],
      prompt: "",
    };

    expect(
      resolveDraftForWorkstationTypeChange(
        invokeDraft,
        "MODEL_WORKSTATION",
        baseValues,
      ),
    ).toEqual({
      ...invokeDraft,
      workstationType: "MODEL_WORKSTATION",
      operation: "",
      operationBindings: [],
      prompt: "Loaded prompt",
      workerName: "reviewer",
    });
  });
});
