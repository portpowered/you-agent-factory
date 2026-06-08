import {
  createEmptyFactoryGraphAddModelOperationDraft,
  type FactoryGraphAddModelOperationDraft,
} from "../lib/factory-graph-add-model-operation-draft";

export function buildOperationDraft(): FactoryGraphAddModelOperationDraft {
  const operation = createEmptyFactoryGraphAddModelOperationDraft();
  operation.name = "TTS";
  operation.inputs[0] = {
    contentTypes: ["TEXT"],
    name: "text",
    required: true,
  };
  operation.outputs[0] = {
    contentTypes: ["AUDIO"],
    name: "audio",
    required: false,
  };
  return operation;
}
