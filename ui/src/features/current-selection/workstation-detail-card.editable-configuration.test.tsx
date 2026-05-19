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
    expect(screen.queryByLabelText("Model")).toBeNull();

    fireEvent.click(toggle);

    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText("Model")).toBeTruthy();
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
    expect(screen.getByLabelText("Model")).toBeTruthy();

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
      within(editableConfigurationSection()).queryByLabelText("Model"),
    ).toBeNull();
  });

  it("renders editable controls with local-draft and validation states", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onModelChange = vi.fn();
    const onPromptChange = vi.fn();
    const onPromptFileChange = vi.fn();

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
          onModelChange,
          onPromptChange,
          onPromptFileChange,
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

    fireEvent.change(screen.getByLabelText("Model"), {
      target: { value: "gpt-5.6" },
    });
    fireEvent.change(screen.getByLabelText("Template"), {
      target: { value: "prompts/review-v2.md" },
    });
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated prompt" },
    });

    expect(onModelChange).toHaveBeenCalledWith("gpt-5.6");
    expect(onPromptFileChange).toHaveBeenCalledWith("prompts/review-v2.md");
    expect(onPromptChange).toHaveBeenCalledWith("Updated prompt");
  });

  it("disables the model field when the selected workstation shares its worker", () => {
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
            isModelEditable: false,
            model: "gpt-5.5",
            modelEditBlockedReason:
              'Model edits are disabled here because worker "processor" is shared with "Review" and "Plan".',
            prompt: "Review the latest story changes before approval.",
            promptFile: "prompts/review.md",
            workerName: "processor",
            workstationName: "Review",
          },
          isDirty: false,
          isModelEditable: false,
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

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand",
      }),
    );

    expect(
      screen.getByLabelText("Model").getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      screen.getByText(
        'Model edits are disabled here because worker "processor" is shared with "Review" and "Plan".',
      ),
    ).toBeTruthy();
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
