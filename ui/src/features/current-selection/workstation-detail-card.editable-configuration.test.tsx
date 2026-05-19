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

function buildReadyEditableConfigurationState(overrides?: {
  prompt?: string;
  validationErrors?: { prompt?: string; workerName?: string };
  workerName?: string;
  workerOptionsState?: { status: "ready"; options: string[] } | { message: string; status: "empty" | "error" };
}) {
  return {
    draft: {
      prompt:
        overrides?.prompt ?? "Review the latest story changes before approval.",
      workerName: overrides?.workerName ?? "reviewer",
    },
    hasValidationErrors: Boolean(
      overrides?.validationErrors?.prompt || overrides?.validationErrors?.workerName,
    ),
    initialValues: {
      prompt: "Review the latest story changes before approval.",
      workerName: "reviewer",
      workerOptions: ["reviewer", "planner"],
      workstationName: "Review",
    },
    isDirty: Boolean(
      overrides?.validationErrors?.prompt ||
        overrides?.validationErrors?.workerName ||
        overrides?.prompt,
    ),
    markChangesSaved: vi.fn(),
    onPromptChange: vi.fn(),
    onWorkerChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: null,
    status: "ready" as const,
    validationErrors: overrides?.validationErrors ?? {},
    workerOptionsState:
      overrides?.workerOptionsState ?? {
        options: ["reviewer", "planner"],
        status: "ready" as const,
      },
  };
}

describe("WorkstationDetailCard editable configuration", () => {
  it("starts collapsed and expands with accessible disclosure behavior", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState()}
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
        editableConfigurationState={buildReadyEditableConfigurationState()}
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
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "Plan the next change.",
          workerName: "planner",
        })}
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
    const onWorkerChange = vi.fn();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          ...buildReadyEditableConfigurationState({
            prompt: "",
            validationErrors: {
              prompt: "Enter a prompt before saving this workstation.",
            },
          }),
          onPromptChange,
          onWorkerChange,
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
    expect(screen.getByLabelText("Worker").tagName).toBe("SELECT");
    expect(screen.queryByLabelText("Model")).toBeNull();
    expect(screen.queryByLabelText("Template")).toBeNull();

    fireEvent.change(screen.getByLabelText("Worker"), {
      target: { value: "planner" },
    });
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated prompt" },
    });

    expect(onWorkerChange).toHaveBeenCalledWith("planner");
    expect(onPromptChange).toHaveBeenCalledWith("Updated prompt");
  });

  it("renders explicit worker empty and stale-selection states", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          workerOptionsState: {
            message:
              "No current workers are available for this workstation. Add a worker to the factory before editing this field.",
            status: "empty",
          },
        })}
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
        "No current workers are available for this workstation. Add a worker to the factory before editing this field.",
      ),
    ).toBeTruthy();
    expect(screen.queryByLabelText("Worker")).toBeNull();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          validationErrors: {
            workerName:
              "The selected worker is no longer available. Choose another worker before saving this workstation.",
          },
          workerName: "missing-worker",
          workerOptionsState: {
            message:
              "The selected workstation references a worker that is no longer available in the current factory definition. Reload current selection and choose another worker.",
            status: "error",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(
      screen.getByText(
        "Worker selection unavailable. The selected workstation references a worker that is no longer available in the current factory definition. Reload current selection and choose another worker.",
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
            "This running factory definition does not expose editable worker and prompt values for the selected workstation.",
          status: "empty",
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(
      screen.getByText(
        "This running factory definition does not expose editable worker and prompt values for the selected workstation.",
      ),
    ).toBeTruthy();
  });
});
