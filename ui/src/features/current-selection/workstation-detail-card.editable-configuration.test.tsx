import { render, screen, within } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { WorkstationDetailCard } from "./workstation-detail-card";

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

describe("WorkstationDetailCard editable configuration", () => {
  it("renders editable workstation prompt, model, template, and worker values", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          status: "ready",
          values: {
            model: "gpt-5.5",
            prompt: "Review the latest story changes before approval.",
            promptFile: "prompts/review.md",
            workerName: "reviewer",
            workstationName: "Review",
          },
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const editableSection = screen
      .getByRole("heading", { name: "Editable configuration" })
      .closest("section");
    if (!editableSection) {
      throw new Error("expected editable configuration section");
    }

    expect(within(editableSection).getByText("Model")).toBeTruthy();
    expect(within(editableSection).getByText("gpt-5.5")).toBeTruthy();
    expect(within(editableSection).getByText("Template")).toBeTruthy();
    expect(within(editableSection).getByText("prompts/review.md")).toBeTruthy();
    expect(within(editableSection).getByText("Worker")).toBeTruthy();
    expect(within(editableSection).getByText("reviewer")).toBeTruthy();
    expect(within(editableSection).getByText("Prompt")).toBeTruthy();
    expect(
      within(editableSection).getByText(
        "Review the latest story changes before approval.",
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

    expect(
      screen.getByText("Loading the current factory definition for this workstation."),
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
