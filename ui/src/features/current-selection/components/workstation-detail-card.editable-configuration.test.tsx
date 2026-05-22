import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { WorkstationDetailCard } from "./workstation-detail-card";

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

function editableConfigurationSection() {
  const heading = screen
    .getAllByRole("heading", { name: "Configuration" })
    .at(-1);
  const section = heading?.closest("section");
  if (!section) {
    throw new Error("expected editable configuration section");
  }

  return section;
}

function buildReadyEditableConfigurationState(overrides?: {
  promptDiagnostics?: Array<{
    endOffset?: number;
    kind: string;
    message: string;
    path?: string;
    sourceText?: string;
    startOffset?: number;
  }>;
  prompt?: string;
  promptHelpState?:
    | {
        contract: {
          availableVariables: Array<{
            category: string;
            description: string;
            example: string;
            path: string;
          }>;
          inputCount: number;
          unavailableAccessPatterns: Array<{
            example: string;
            path: string;
            reason: string;
          }>;
        };
        status: "ready";
      }
    | { message: string; status: "empty" }
    | { errorMessage: string; status: "error" }
    | { status: "loading" };
  promptValidationState?:
    | { status: "idle" }
    | { status: "loading" }
    | { errorMessage: string; status: "error" }
    | {
        diagnostics: Array<{
          endOffset?: number;
          kind: string;
          message: string;
          path?: string;
          sourceText?: string;
          startOffset?: number;
        }>;
        result: {
          diagnostics: Array<{
            endOffset?: number;
            kind: string;
            message: string;
            path?: string;
            sourceText?: string;
            startOffset?: number;
          }>;
          valid: boolean;
        };
        status: "ready";
      };
  validationErrors?: { prompt?: string; workerName?: string };
  workerName?: string;
  workerOptionsState?:
    | { status: "ready"; options: string[] }
    | { message: string; status: "empty" | "error" };
}) {
  return {
    draft: {
      prompt:
        overrides?.prompt ?? "Review the latest story changes before approval.",
      runnerName: "gemini",
      workerName: overrides?.workerName ?? "reviewer",
    },
    hasValidationErrors: Boolean(
      overrides?.validationErrors?.prompt ||
        overrides?.validationErrors?.workerName,
    ),
    initialValues: {
      effectiveRunnerName: "gemini",
      factoryRunnerName: "codex",
      prompt: "Review the latest story changes before approval.",
      runnerName: "gemini",
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
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
    onRunnerChange: vi.fn(),
    onWorkerChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: null,
    promptDiagnostics: overrides?.promptDiagnostics ?? [],
    promptHelpState: overrides?.promptHelpState ?? {
      contract: {
        availableVariables: [
          {
            category: "ROOT",
            description: "The current work item identifier.",
            example: "{{ .WorkID }}",
            path: ".WorkID",
          },
          {
            category: "INPUT",
            description: "Payload for the first authored input.",
            example: "{{ (index .Inputs 0).Payload }}",
            path: ".Inputs[0].Payload",
          },
        ],
        inputCount: 1,
        unavailableAccessPatterns: [
          {
            example: "{{ (index .Inputs 1).Payload }}",
            path: ".Inputs[1].Payload",
            reason: "Only input 0 is available for this workstation.",
          },
        ],
      },
      status: "ready" as const,
    },
    promptValidationState:
      overrides?.promptValidationState ??
      (overrides?.promptDiagnostics && overrides.promptDiagnostics.length > 0
        ? {
            diagnostics: overrides.promptDiagnostics,
            result: {
              diagnostics: overrides.promptDiagnostics,
              valid: false,
            },
            status: "ready" as const,
          }
        : {
            result: {
              diagnostics: [],
              valid: true,
            },
            diagnostics: [],
            status: "ready" as const,
          }),
    status: "ready" as const,
    validationErrors: overrides?.validationErrors ?? {},
    workerOptionsState: overrides?.workerOptionsState ?? {
      options: ["reviewer", "planner"],
      status: "ready" as const,
    },
  };
}

describe("WorkstationDetailCard editable configuration", () => {
  it("supports keyboard disclosure toggling for the editable configuration section", async () => {
    const user = userEvent.setup();
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
      name: "Expand editable configuration",
    });

    toggle.focus();
    await user.keyboard("{Enter}");

    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText("Worker")).toBeTruthy();
    expect(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Collapse editable configuration",
      }),
    ).toBeTruthy();

    await user.keyboard(" ");

    expect(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    ).toBeTruthy();
    expect(screen.queryByLabelText("Worker")).toBeNull();
  });

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
      name: "Expand editable configuration",
    });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText("Worker")).toBeNull();
    expect(screen.queryByLabelText("Prompt")).toBeNull();

    fireEvent.click(toggle);

    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText("Worker")).toBeTruthy();
    expect(
      screen.getByLabelText("Prompt").getAttribute("data-monaco-editor"),
    ).toBe("workstation-prompt");
    expect(
      screen.getByDisplayValue(
        "Review the latest story changes before approval.",
      ),
    ).toBeTruthy();
    expect(
      within(editableConfigurationSection())
        .getByRole("button", { name: "Collapse editable configuration" })
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
        name: "Expand editable configuration",
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
        name: "Expand editable configuration",
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
        name: "Expand editable configuration",
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

  it("shows prompt variable help inline from the current workstation contract", () => {
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

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Open prompt variable help" }),
    );

    expect(
      screen.getByText(
        "Autocomplete is ready with 2 variables for 1 authored input.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Type inside {{ ... }} to see suggestions, or open Monaco completion manually anywhere in the prompt editor.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("This workstation exposes 1 authored input."),
    ).toBeTruthy();
    expect(screen.getByText("Prompt variable help")).toBeTruthy();
    expect(screen.getByText("Available variables")).toBeTruthy();
    expect(screen.getByText(".WorkID")).toBeTruthy();
    expect(screen.getByText("{{ .WorkID }}")).toBeTruthy();
    expect(screen.getByText("Unavailable access patterns")).toBeTruthy();
    expect(screen.getByText(".Inputs[1].Payload")).toBeTruthy();
    expect(
      screen.getByText("Only input 0 is available for this workstation."),
    ).toBeTruthy();
  });

  it("renders inline prompt diagnostics with squiggle feedback for invalid variables", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "Use {{ (index .Inputs 1).Payload }} now.",
          promptDiagnostics: [
            {
              endOffset: 33,
              kind: "UNAVAILABLE_VARIABLE",
              message: "Only input 0 is available.",
              path: ".Inputs[1]",
              sourceText: "(index .Inputs 1)",
              startOffset: 7,
            },
          ],
          validationErrors: {
            prompt:
              "Resolve the highlighted prompt diagnostics before saving this workstation.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    expect(screen.getByText("Prompt diagnostics")).toBeTruthy();
    expect(
      screen.getAllByText(
        "Resolve the highlighted prompt diagnostics before saving this workstation.",
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText(".Inputs[1]")).toBeTruthy();
    expect(screen.getAllByText("(index .Inputs 1)").length).toBeGreaterThan(0);
  });

  it("does not render a squiggle overlay when the prompt has no diagnostics", () => {
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

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    expect(editableConfigurationSection().querySelector("mark")).toBeNull();
  });

  it("labels syntax diagnostics separately from variable-access diagnostics", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "Use {{ if .WorkID }} now.",
          promptDiagnostics: [
            {
              endOffset: 18,
              kind: "SYNTAX_ERROR",
              message: "Unexpected EOF in if block.",
              sourceText: "{{ if .WorkID }}",
              startOffset: 5,
            },
          ],
          validationErrors: {
            prompt:
              "Resolve the highlighted prompt diagnostics before saving this workstation.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    expect(
      screen.getByText("Template syntax: Unexpected EOF in if block."),
    ).toBeTruthy();
    expect(
      screen.queryByText("Variable access: Unexpected EOF in if block."),
    ).toBeNull();
  });

  it("keeps the squiggle aligned for runtime-generated diagnostics beyond column one", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "x{{ index .Context.Env 0 }} now",
          promptDiagnostics: [
            {
              endOffset: 24,
              kind: "INVALID_VARIABLE",
              message:
                "Template execution would fail: value has type int; should be string.",
              path: ".Context.Env",
              sourceText: "index .Context.Env 0",
              startOffset: 5,
            },
          ],
          validationErrors: {
            prompt:
              "Resolve the highlighted prompt diagnostics before saving this workstation.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    const squiggle = editableConfigurationSection().querySelector("mark");
    expect(squiggle?.textContent).toBe("index .Context.Env 0");
  });

  it("renders explicit prompt-validation loading and error states", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          promptValidationState: { status: "loading" },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    expect(
      screen.getByText("Validating prompt variables for the current draft."),
    ).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          promptValidationState: {
            errorMessage: "Prompt validation API unavailable.",
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
        "Prompt validation unavailable. Prompt validation API unavailable.",
      ),
    ).toBeTruthy();
  });

  it("merges overlapping diagnostic ranges into one visible squiggle", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "Use {{ .Prompt }} now.",
          promptDiagnostics: [
            {
              endOffset: 17,
              kind: "INVALID_VARIABLE",
              message: "Prompt root is invalid.",
              path: ".Prompt",
              sourceText: "{{ .Prompt }}",
              startOffset: 5,
            },
            {
              endOffset: 15,
              kind: "INVALID_VARIABLE",
              message: "Prompt access is invalid.",
              path: ".Prompt",
              sourceText: ".Prompt",
              startOffset: 9,
            },
          ],
          validationErrors: {
            prompt:
              "Resolve the highlighted prompt diagnostics before saving this workstation.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    const squiggles = editableConfigurationSection().querySelectorAll("mark");
    expect(squiggles).toHaveLength(1);
    expect(squiggles[0]?.textContent).toBe("{{ .Prompt }}");
  });

  it("uses byte offsets correctly when diagnostics begin after multibyte characters", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "😀 {{ .Prompt }}",
          promptDiagnostics: [
            {
              endOffset: 18,
              kind: "INVALID_VARIABLE",
              message: "Prompt root is invalid.",
              path: ".Prompt",
              sourceText: "{{ .Prompt }}",
              startOffset: 6,
            },
          ],
          validationErrors: {
            prompt:
              "Resolve the highlighted prompt diagnostics before saving this workstation.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    const squiggle = editableConfigurationSection().querySelector("mark");
    expect(squiggle?.textContent).toBe("{{ .Prompt }}");
  });

  it("clamps diagnostic offsets that start at byte one or extend past the prompt end", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "Prompt",
          promptDiagnostics: [
            {
              endOffset: 999,
              kind: "INVALID_VARIABLE",
              message: "Whole prompt is invalid.",
              sourceText: "Prompt",
              startOffset: 1,
            },
          ],
          validationErrors: {
            prompt:
              "Resolve the highlighted prompt diagnostics before saving this workstation.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    const squiggle = editableConfigurationSection().querySelector("mark");
    expect(squiggle?.textContent).toBe("Prompt");
  });

  it("falls back to source-text matching when authoritative offsets are unavailable", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt:
            "Use {{ .Prompt }} first and {{ .Prompt }} second for review.",
          promptDiagnostics: [
            {
              kind: "INVALID_VARIABLE",
              message: "First prompt access is invalid.",
              sourceText: "{{ .Prompt }}",
            },
            {
              kind: "INVALID_VARIABLE",
              message: "Second prompt access is invalid.",
              sourceText: "{{ .Prompt }}",
            },
          ],
          validationErrors: {
            prompt:
              "Resolve the highlighted prompt diagnostics before saving this workstation.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    const squiggles = Array.from(
      editableConfigurationSection().querySelectorAll("mark"),
    ).map((element) => element.textContent);
    expect(squiggles).toEqual(["{{ .Prompt }}", "{{ .Prompt }}"]);
  });

  it("renders loading, empty, and error prompt variable help states explicitly", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          promptHelpState: { status: "loading" },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Open prompt variable help" }),
    );

    expect(
      screen.getAllByText(
        "Loading available prompt variables for this workstation.",
      ).length,
    ).toBeGreaterThan(0);

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          promptHelpState: {
            message:
              "No prompt variable help is available for this workstation.",
            status: "empty",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(
      screen.getAllByText(
        "No prompt variable help is available for this workstation.",
      ).length,
    ).toBeGreaterThan(0);

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          promptHelpState: {
            errorMessage: "Current named factory workstation not found.",
            status: "error",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText(
        "Prompt variable help unavailable. Current named factory workstation not found.",
      ).length,
    ).toBeGreaterThan(0);
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
        name: "Expand editable configuration",
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
        name: "Expand editable configuration",
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
