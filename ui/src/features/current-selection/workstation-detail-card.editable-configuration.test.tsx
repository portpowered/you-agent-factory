import { fireEvent, render, screen, within } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { WorkstationDetailCard } from "./workstation-detail-card";

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

function editableConfigurationSection() {
  const heading = screen
    .getAllByRole("heading", { name: "Editable configuration" })
    .at(-1);
  const section = heading?.closest("section");
  if (!section) {
    throw new Error("expected editable configuration section");
  }

  return section;
}

describe("WorkstationDetailCard editable configuration", () => {
  it("starts collapsed and expands with accessible disclosure behavior", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          draft: {
            model: "gpt-5.5",
            prompt: "Review the latest story changes before approval.",
            promptFile: "prompts/review.md",
          },
          hasValidationErrors: false,
          initialValues: {
            isModelEditable: true,
            model: "gpt-5.5",
            modelEditBlockedReason: null,
            prompt: "Review the latest story changes before approval.",
            promptFile: "prompts/review.md",
            workerName: "reviewer",
            workstationName: "Review",
          },
          isDirty: false,
          isModelEditable: true,
          onModelChange: vi.fn(),
          onPromptChange: vi.fn(),
          onPromptFileChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: null,
          status: "ready",
          validationErrors: {},
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const toggle = within(editableConfigurationSection()).getByRole("button", {
      name: "Expand",
    });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText("Worker")).toBeNull();
    expect(screen.queryByLabelText("Prompt")).toBeNull();

    fireEvent.click(toggle);

    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText("Worker")).toBeTruthy();
    expect(
      screen.getByDisplayValue(
        "Review the latest story changes before approval.",
      ),
    ).toBeTruthy();
    expect(
      within(editableConfigurationSection())
        .getByRole("button", { name: "Collapse" })
        .getAttribute("aria-controls"),
    ).toBeTruthy();
  });

  it("resets the disclosure to collapsed when the selected workstation changes", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const reviewNode = snapshot.topology.workstation_nodes_by_id.review;
    const planNode = snapshot.topology.workstation_nodes_by_id.plan;

    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          draft: {
            model: "gpt-5.5",
            prompt: "Review the latest story changes before approval.",
            promptFile: "prompts/review.md",
          },
          hasValidationErrors: false,
          initialValues: {
            isModelEditable: true,
            model: "gpt-5.5",
            modelEditBlockedReason: null,
            prompt: "Review the latest story changes before approval.",
            promptFile: "prompts/review.md",
            workerName: "reviewer",
            workstationName: "Review",
          },
          isDirty: false,
          isModelEditable: true,
          onModelChange: vi.fn(),
          onPromptChange: vi.fn(),
          onPromptFileChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: null,
          status: "ready",
          validationErrors: {},
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={reviewNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand",
      }),
    );
    expect(screen.getByLabelText("Worker")).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          draft: {
            model: "gpt-5.5",
            prompt: "Plan the next change.",
            promptFile: "prompts/plan.md",
          },
          hasValidationErrors: false,
          initialValues: {
            isModelEditable: true,
            model: "gpt-5.5",
            modelEditBlockedReason: null,
            prompt: "Plan the next change.",
            promptFile: "prompts/plan.md",
            workerName: "planner",
            workstationName: "Plan",
          },
          isDirty: false,
          isModelEditable: true,
          onModelChange: vi.fn(),
          onPromptChange: vi.fn(),
          onPromptFileChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: null,
          status: "ready",
          validationErrors: {},
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={planNode}
      />,
    );

    expect(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand",
      }),
    ).toBeTruthy();
    expect(
      within(editableConfigurationSection()).queryByLabelText("Worker"),
    ).toBeNull();
  });

  it("renders only worker and prompt controls in the editable form", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onPromptChange = vi.fn();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          draft: {
            model: "",
            prompt: "",
            promptFile: "   ",
          },
          hasValidationErrors: true,
          initialValues: {
            isModelEditable: true,
            model: "gpt-5.5",
            modelEditBlockedReason: null,
            prompt: "Review the latest story changes before approval.",
            promptFile: "prompts/review.md",
            workerName: "reviewer",
            workstationName: "Review",
          },
          isDirty: true,
          isModelEditable: true,
          onModelChange: vi.fn(),
          onPromptChange,
          onPromptFileChange: vi.fn(),
          overwriteFieldNames: [],
          pendingFactoryDefinition: null,
          status: "ready",
          validationErrors: {
            model: "Enter a model before saving this workstation.",
            prompt: "Enter a prompt before saving this workstation.",
            promptFile:
              "Template paths cannot be only whitespace. Clear the field to remove the template.",
          },
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand",
      }),
    );

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(
      screen.getByText(
        "Resolve the highlighted fields before saving this workstation.",
      ),
    ).toBeTruthy();
    expect(screen.getByLabelText("Worker")).toBeTruthy();
    expect(screen.getByLabelText("Worker").getAttribute("readonly")).not.toBeNull();
    expect(screen.queryByLabelText("Model")).toBeNull();
    expect(screen.queryByLabelText("Template")).toBeNull();

    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated prompt" },
    });

    expect(onPromptChange).toHaveBeenCalledWith("Updated prompt");
  });

  it("renders explicit loading, error, and empty editable-configuration states", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{ status: "loading" }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand",
      }),
    );

    expect(
      screen.getByText(
        "Loading the current factory definition for this workstation.",
      ),
    ).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          errorMessage: "The current factory API rejected the request.",
          status: "error",
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(
      screen.getByText(
        "Editable configuration unavailable. The current factory API rejected the request.",
      ),
    ).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          message:
            "This running factory definition does not expose editable prompt, model, and template values for the selected workstation.",
          status: "empty",
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(
      screen.getByText(
        "This running factory definition does not expose editable prompt, model, and template values for the selected workstation.",
      ),
    ).toBeTruthy();
  });
});
