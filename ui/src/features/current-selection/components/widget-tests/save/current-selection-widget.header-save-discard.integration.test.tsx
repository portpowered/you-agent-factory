import "../../../../../testing/vitest-dom-capabilities.setup";

import "@testing-library/jest-dom/vitest";
import { fireEvent, screen, within } from "@testing-library/react";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../../../current-factory-definition/hooks/useFactoryDocumentSave";
import {
  currentSelectionHeaderActionSection,
  expandDetailCardWorkerConfiguration,
  expandDetailCardWorkstationConfiguration,
} from "../../../base/components/detail-card/detail-card-save-test-helpers";
import {
  buildDetailCardCurrentSelection,
  buildDetailCardEditableFactoryDocument,
  buildDetailCardFactoryDocumentQueryResult,
  buildDetailCardFactoryDocumentSaveHookReturn,
  DETAIL_CARD_NOW,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { resetSelectionHistoryStore } from "../../../state/selectionHistoryStore";
import { useCurrentWorkstationPromptTemplateValidation } from "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation";
import { CurrentSelectionWidget } from "../../widget/current-selection-widget";
import { renderWithQueryClient } from "../../widget/current-selection-widget-test-utils";

const saveCurrentFactoryMutation = vi.fn();

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

vi.mock(
  "../../../workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation",
  () => ({
    useCurrentWorkstationPromptTemplateValidation: vi.fn(),
  }),
);

function editableConfigurationSection(headingName: string) {
  const section = screen
    .getAllByRole("heading", { name: headingName })
    .at(-1)
    ?.closest("section");
  if (!section) {
    throw new Error(`expected ${headingName} section`);
  }

  return section;
}

function assertHeaderSaveDiscardOnly({
  configurationHeading,
  discardLabel = "Discard local changes",
  saveLabel,
}: {
  configurationHeading: string;
  discardLabel?: string;
  saveLabel: string;
}) {
  const headerActions = currentSelectionHeaderActionSection();
  const saveButtons = within(headerActions).getAllByRole("button", {
    name: saveLabel,
  });
  const discardButtons = within(headerActions).getAllByRole("button", {
    name: discardLabel,
  });
  expect(saveButtons).toHaveLength(1);
  expect(discardButtons).toHaveLength(1);

  const configurationSection =
    editableConfigurationSection(configurationHeading);
  expect(
    within(configurationSection).queryByRole("button", { name: saveLabel }),
  ).toBeNull();
  expect(
    within(configurationSection).queryByRole("button", {
      name: discardLabel,
    }),
  ).toBeNull();
}

describe("CurrentSelectionWidget header save and discard integration", () => {
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
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockReturnValue({
      data: {
        diagnostics: [],
        valid: true,
      },
      error: null,
      isError: false,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("keeps workstation save and discard in the header only after dirty edits", () => {
    const selectedNode =
      semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          selectedNode,
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkstationConfiguration();
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated prompt {{ .WorkID }}." },
    });

    assertHeaderSaveDiscardOnly({
      configurationHeading: "Configuration",
      saveLabel: "Save changes",
    });
  });

  it("keeps worker save and discard in the header only after dirty edits", () => {
    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildDetailCardCurrentSelection({
          currentFactoryDefinition: buildDetailCardEditableFactoryDocument(),
          selectedWorkerName: "reviewer",
          selection: { kind: "worker", workerName: "reviewer" },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
      />,
    );

    expandDetailCardWorkerConfiguration();
    fireEvent.change(screen.getByLabelText("Model"), {
      target: { value: "gpt-5.9" },
    });

    assertHeaderSaveDiscardOnly({
      configurationHeading: "Worker configuration",
      saveLabel: "Save worker",
    });
  });
});
