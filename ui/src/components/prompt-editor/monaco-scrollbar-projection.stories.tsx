import { useState } from "react";

import "../../styles.css";
import {
  CURRENT_SELECTION_FACTORY_DOC_MODEL_PATH,
  CURRENT_SELECTION_WORKSTATION_PROMPT_MODEL_PATH,
  MonacoGuardSelectorEditor,
  MonacoPromptEditor,
  MonacoTextEditor,
} from ".";
import type { PromptEditorAutocompleteState } from "./prompt-editor-types";

const readyAutocompleteState: PromptEditorAutocompleteState = {
  contract: {
    availableVariables: [],
    inputCount: 0,
    unavailableAccessPatterns: [],
  },
  status: "ready",
};

const overflowingEditorValue = Array.from(
  { length: 180 },
  (_, index) => `Line ${index + 1}: scrollbar projection verification content.`,
).join("\n");
const guardSelectorModelPath =
  "inmemory://model/scrollbar-projection/guard-selector";

function MonacoScrollbarProjectionStory() {
  const [prompt, setPrompt] = useState(overflowingEditorValue);
  const [documentValue, setDocumentValue] = useState(overflowingEditorValue);
  const [guardSelector, setGuardSelector] = useState(".Name");

  return (
    <main
      className="grid gap-6 bg-surface p-6 text-on-surface"
      style={{ width: "960px" }}
    >
      <section className="grid gap-2">
        <h2 className="m-0 text-xl">Prompt editor</h2>
        <MonacoPromptEditor
          ariaLabel="Scrollbar projection prompt editor"
          autocompleteState={readyAutocompleteState}
          height="10rem"
          loadingMessage="Loading prompt editor"
          modelPath={`${CURRENT_SELECTION_WORKSTATION_PROMPT_MODEL_PATH}/scrollbar-projection`}
          onChange={setPrompt}
          startupErrorMessage="Prompt editor failed to load"
          value={prompt}
        />
      </section>
      <section className="grid gap-2">
        <h2 className="m-0 text-xl">Document editor</h2>
        <MonacoTextEditor
          ariaLabel="Scrollbar projection document editor"
          height="10rem"
          loadingMessage="Loading document editor"
          modelPath={`${CURRENT_SELECTION_FACTORY_DOC_MODEL_PATH}/scrollbar-projection`}
          onChange={setDocumentValue}
          startupErrorMessage="Document editor failed to load"
          value={documentValue}
        />
      </section>
      <section className="grid gap-2">
        <h2 className="m-0 text-xl">Guard selector editor</h2>
        <MonacoGuardSelectorEditor
          ariaLabel="Scrollbar projection guard selector editor"
          height="2.75rem"
          loadingMessage="Loading guard selector editor"
          modelPath={guardSelectorModelPath}
          onChange={setGuardSelector}
          startupErrorMessage="Guard selector editor failed to load"
          value={guardSelector}
        />
      </section>
    </main>
  );
}

export default {
  title: "you-agent-factory/Components/Prompt Editor",
  tags: ["test"],
};

export const MonacoScrollbarProjection = {
  render: () => <MonacoScrollbarProjectionStory />,
};
