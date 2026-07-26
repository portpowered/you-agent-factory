import "../../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, screen, waitFor } from "@testing-library/react";

import {
  getCurrentFactoryWorkstationPromptTemplateContract,
  type PromptTemplateContract,
  type PromptTemplateValidationResult,
  validateCurrentFactoryWorkstationPromptTemplate,
} from "../../../../../api/current-factory-prompt-template";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../../../current-factory-definition/hooks/useFactoryDocumentSave";
import {
  buildDetailCardEditableFactoryDocument,
  buildDetailCardFactoryDocumentQueryResult,
  buildDetailCardFactoryDocumentSaveHookReturn,
  buildDetailCardWorkstationNodeSelection,
  DETAIL_CARD_NOW,
  expandDetailCardWorkstationConfiguration,
  workstationFooterSaveButton,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { resetSelectionHistoryStore } from "../../../state/selectionHistoryStore";
import { CurrentSelectionWidget } from "../../widget/current-selection-widget";
import { renderWithQueryClient } from "../../widget/current-selection-widget-test-utils";

const saveCurrentFactoryMutation = vi.fn();

const promptTemplateContract: PromptTemplateContract = {
  availableVariables: [
    {
      category: "ROOT",
      description: "The current work item identifier.",
      example: "{{ .WorkID }}",
      path: ".WorkID",
    },
  ],
  inputCount: 1,
  unavailableAccessPatterns: [],
};

const validPromptValidation: PromptTemplateValidationResult = {
  diagnostics: [],
  valid: true,
};

vi.mock("../../../../../api/current-factory-prompt-template", async () => {
  const actual = await vi.importActual<
    typeof import("../../../../../api/current-factory-prompt-template")
  >("../../../../../api/current-factory-prompt-template");

  return {
    ...actual,
    getCurrentFactoryWorkstationPromptTemplateContract: vi.fn(),
    validateCurrentFactoryWorkstationPromptTemplate: vi.fn(),
  };
});

vi.mock(
  "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../../../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

describe("CurrentSelectionWidget prompt-edit save enablement", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
    saveCurrentFactoryMutation.mockReset();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildDetailCardFactoryDocumentQueryResult(
        buildDetailCardEditableFactoryDocument(),
      ),
    );
    vi.mocked(useFactoryDocumentSave).mockReturnValue(
      buildDetailCardFactoryDocumentSaveHookReturn(
        saveCurrentFactoryMutation,
      ) as never,
    );
    vi.mocked(
      getCurrentFactoryWorkstationPromptTemplateContract,
    ).mockImplementation(async () => promptTemplateContract);
    vi.mocked(
      validateCurrentFactoryWorkstationPromptTemplate,
    ).mockImplementation(async (_workstationName, prompt) => {
      if (
        prompt.includes("{{ if .WorkID }}") &&
        !prompt.includes("{{ end }}")
      ) {
        return {
          diagnostics: [
            {
              endOffset: prompt.length,
              kind: "SYNTAX_ERROR",
              message: "line 1: unexpected EOF",
              startOffset: 0,
            },
          ],
          valid: false,
        };
      }

      return {
        ...validPromptValidation,
        diagnostics: [],
        valid: prompt.trim().length > 0,
      };
    });
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("enables save and submits after a valid prompt-only edit", async () => {
    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardWorkstationNodeSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    const saveButton = workstationFooterSaveButton();
    expect(saveButton.getAttribute("disabled")).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated review instructions for save." },
    });

    await waitFor(() => {
      expect(saveButton.getAttribute("disabled")).toBeNull();
    });

    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });
  });

  it("re-enables save after correcting a template syntax typo", async () => {
    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardWorkstationNodeSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();

    const saveButton = workstationFooterSaveButton();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "{{ if .WorkID }}" },
    });

    await waitFor(() => {
      expect(saveButton.getAttribute("disabled")).not.toBeNull();
      expect(screen.getByText(/line 1: unexpected EOF/)).toBeTruthy();
    });

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "{{ if .WorkID }}{{ end }}" },
    });

    await waitFor(() => {
      expect(saveButton.getAttribute("disabled")).toBeNull();
    });

    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(saveCurrentFactoryMutation).toHaveBeenCalledTimes(1);
    });
  });
});
