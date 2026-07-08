import { render, screen } from "@testing-library/react";

import { MonacoGuardSelectorEditor } from "./monaco-guard-selector-editor";
import { MonacoPromptEditor } from "./monaco-prompt-editor";
import { PromptEditorDiagnosticsPanel } from "./prompt-editor-diagnostics-panel";
import { VerticalResizableWidth } from "./vertical-resizable-width";

const readyAutocompleteState = {
  contract: {
    availableVariables: [],
    inputCount: 1,
    unavailableAccessPatterns: [],
  },
  status: "ready" as const,
};

const diagnosticsLabels = {
  diagnosticsHeading: "Prompt diagnostics",
  diagnosticsSummary: "Resolve the highlighted prompt diagnostics.",
  validationErrorPrefix: "Prompt validation unavailable.",
  validationLoading: "Validating prompt variables.",
  variableDiagnosticLabel: "Variable access",
};

describe("prompt-editor neutral surface role behavior", () => {
  it("renders prompt and guard selector editor shells with outline borders", async () => {
    render(
      <MonacoPromptEditor
        ariaLabel="Prompt"
        autocompleteState={readyAutocompleteState}
        loadingMessage="Loading prompt editor."
        modelPath="inmemory://model/test/prompt-editor-surface-roles"
        onChange={() => {}}
        startupErrorMessage="Prompt editor failed."
        value="Initial prompt"
      />,
    );

    const promptEditor = screen.getByLabelText("Prompt");
    expect(promptEditor.parentElement?.className).toContain("border-outline");

    render(
      <MonacoGuardSelectorEditor
        ariaLabel="Field selector"
        loadingMessage="Starting selector editor."
        modelPath="inmemory://model/test/guard-selector-surface-roles"
        onChange={() => {}}
        startupErrorMessage="Selector editor failed."
        value=".Name"
      />,
    );

    const selectorEditor = screen.getByLabelText("Field selector");
    expect(selectorEditor.parentElement?.className).toContain("border-outline");
  });

  it("renders diagnostic list items through package-backed surface panels", () => {
    render(
      <PromptEditorDiagnosticsPanel
        diagnostics={[
          {
            endOffset: 13,
            kind: "INVALID_VARIABLE",
            message: "Work ID is invalid.",
            path: ".WorkID",
            sourceText: "{{ .WorkID }}",
            startOffset: 1,
          },
        ]}
        id="prompt-diagnostics"
        labels={diagnosticsLabels}
        validationState={{ status: "ready" }}
      />,
    );

    const diagnosticItem = screen
      .getByText("Variable access: Work ID is invalid.")
      .closest("li");

    expect(diagnosticItem?.className).toContain("bg-surface-container-high");
    expect(diagnosticItem?.className).toContain("border-outline");
  });

  it("renders the resize handle with outline role utilities", () => {
    const { container } = render(
      <VerticalResizableWidth resizeHandleLabel="Resize prompt editor height">
        <div>Prompt editor</div>
      </VerticalResizableWidth>,
    );

    const handleIndicator = container.querySelector(
      "[data-prompt-editor-resize-handle='true'] span",
    );

    expect(handleIndicator?.className).toContain("bg-outline");
    expect(handleIndicator?.className).toContain("hover:bg-outline-variant");
  });
});
