import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  mock,
} from "bun:test";

const vi = { fn: mock };

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import {
  semanticWorkflowDashboardSnapshot,
  workstationKindParityDashboardSnapshot,
} from "../../../../../components/dashboard/test-fixtures";
import { selectLabeledComboboxOption } from "../../../../../testing/select-test-helpers";
import { expectNoInlineSaveOutcomesIn } from "../../../base/components/detail-card/current-selection-save-toast-test-helpers";
import { buildDetailCardEditableFactoryDocument } from "../../../base/components/detail-card/detail-card-test-helpers";
import type {
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
} from "../../lib/keys/detail-card-types";
import { WorkstationDetailCard } from "./workstation-detail-card";

const CURRENT_SELECTION_FORM_FIELDS_SELECTOR = ".grid.grid-cols-1.gap-3";

import { EditableWorkstationConfigurationHeaderActions } from "../editable/workstation-save-controls";

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");
const editableConfigurationCoverageTimeoutMs = 240_000;

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value == null) {
    throw new Error(message);
  }

  return value;
}

function currentSelectionHeaderActionSection() {
  const card = screen.getByRole("article", { name: "Current selection" });
  const undoButton = within(card).getByRole("button", {
    name: "Undo selection",
  });
  const actionSection = undoButton.closest(
    "[data-action-row-section='actions']",
  );
  if (!actionSection) {
    throw new Error("expected header action section");
  }

  return actionSection as HTMLElement;
}

function buildWorkstationHeaderActions({
  canDiscard = false,
  canSave,
  onDiscard = vi.fn(),
  onSave = vi.fn(),
  saveState = { status: "idle" },
}: {
  canDiscard?: boolean;
  canSave: boolean;
  onDiscard?: () => void;
  onSave?: () => void;
  saveState?: EditableWorkstationSaveState;
}) {
  return (
    <EditableWorkstationConfigurationHeaderActions
      canDiscard={canDiscard}
      canSave={canSave}
      onDiscard={onDiscard}
      onSave={onSave}
      saveState={saveState}
    />
  );
}

function expandEditableConfiguration() {
  fireEvent.click(
    within(editableConfigurationSection()).getByRole("button", {
      name: "Expand editable configuration",
    }),
  );
}

function promptVariableHelpToggle() {
  return within(editableConfigurationSection()).getByRole("button", {
    name: "Open prompt variable help",
  });
}

function expectHeadingBefore(first: HTMLElement, second: HTMLElement) {
  expect(
    first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
}

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
  behavior?: "STANDARD" | "REPEATER" | "POLLER" | "CRON";
  cron?: {
    expiryWindow: string;
    jitter: string;
    schedule: string;
    triggerAtStart: boolean;
  };
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
  sharedWorkerWorkstationNames?: string[];
  overwriteFieldNames?: EditableWorkstationOverwriteField[];
  initialValuesWorkstationName?: string;
  validationErrors?: {
    behavior?: string;
    cronExpiryWindow?: string;
    cronJitter?: string;
    cronSchedule?: string;
    cronTriggerAtStart?: string;
    prompt?: string;
    workerName?: string;
  };
  workerName?: string;
  workerOptionsState?:
    | { status: "ready"; options: string[] }
    | { message: string; status: "empty" | "error" };
  workstationType?: "MODEL_WORKSTATION" | "LOGICAL_MOVE";
}) {
  const behavior = overrides?.behavior ?? "STANDARD";
  const cron =
    behavior === "CRON"
      ? (overrides?.cron ?? {
          schedule: "*/5 * * * *",
          triggerAtStart: true,
          jitter: "1s",
          expiryWindow: "30s",
        })
      : null;

  const workstationType = overrides?.workstationType ?? "MODEL_WORKSTATION";

  return {
    draft: {
      behavior,
      cron,
      guards: [],
      inputs: [],
      name: overrides?.initialValuesWorkstationName ?? "Review",
      operation: "",
      operationBindings: [],
      prompt:
        overrides?.prompt ?? "Review the latest story changes before approval.",
      runnerName: "gemini",
      workerName: overrides?.workerName ?? "reviewer",
      workstationType,
    },
    hasValidationErrors: Boolean(
      overrides?.validationErrors?.prompt ||
        overrides?.validationErrors?.workerName ||
        overrides?.validationErrors?.cronSchedule ||
        overrides?.validationErrors?.cronJitter ||
        overrides?.validationErrors?.cronExpiryWindow ||
        overrides?.validationErrors?.cronTriggerAtStart,
    ),
    initialValues: {
      behavior,
      behaviorOptions:
        behavior === "CRON"
          ? ["STANDARD", "REPEATER", "POLLER", "CRON"]
          : ["STANDARD", "REPEATER", "POLLER"],
      cron,
      effectiveRunnerName: "gemini",
      factoryRunnerName: "codex",
      prompt: "Review the latest story changes before approval.",
      resolvedRunnerSelection: {
        runnerId: "gemini",
        source: "workstation",
      },
      runnerName: "gemini",
      runnerOptions: [
        "codex",
        "gemini",
        "kiro",
        "cursor-cli",
        "opencode",
        "pi",
      ],
      runnerSelectionSource: "workstation",
      workerModelProvider: null,
      sharedWorkerWorkstationNamesByWorkerName: {
        planner: ["Plan", "Code"],
        reviewer: overrides?.sharedWorkerWorkstationNames ?? [],
      },
      sharedWorkerWorkstationNames:
        overrides?.sharedWorkerWorkstationNames ?? [],
      workerName: "reviewer",
      workerOptions: ["reviewer", "planner"],
      workerTypeByName: {
        planner: "MODEL_WORKER",
        reviewer: "MODEL_WORKER",
      },
      workstationName: overrides?.initialValuesWorkstationName ?? "Review",
      workstationOptions: ["Plan", "Review"],
      workstationType,
      workstationTypeOptions:
        workstationType === "LOGICAL_MOVE"
          ? ["LOGICAL_MOVE"]
          : ["MODEL_WORKSTATION", "MODEL_INVOKE"],
      modelInvokeWorkerOptions: [],
      modelOperationsByWorkerName: {},
      operation: "",
      operationBindings: [],
      guards: [],
      inputs: [],
    },
    isDirty: Boolean(
      overrides?.behavior ||
        overrides?.cron ||
        overrides?.validationErrors?.prompt ||
        overrides?.validationErrors?.behavior ||
        overrides?.validationErrors?.workerName ||
        overrides?.validationErrors?.cronSchedule ||
        overrides?.validationErrors?.cronJitter ||
        overrides?.validationErrors?.cronExpiryWindow ||
        overrides?.validationErrors?.cronTriggerAtStart ||
        overrides?.prompt,
    ),
    markChangesSaved: vi.fn(),
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    onBehaviorChange: vi.fn(),
    onCronExpiryWindowChange: vi.fn(),
    onCronJitterChange: vi.fn(),
    onCronScheduleChange: vi.fn(),
    onCronTriggerAtStartChange: vi.fn(),
    onNameChange: vi.fn(),
    onPromptChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onGuardsChange: vi.fn(),
    onInputsChange: vi.fn(),
    onRunnerChange: vi.fn(),
    onWorkerChange: vi.fn(),
    workstationOptionsState: {
      options: ["Plan", "Review"],
      status: "ready" as const,
    },
    overwriteFieldNames: overrides?.overwriteFieldNames ?? [],
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: main-branch mergeability change expanded this existing focused test suite past the repo threshold; keep the suite intact until a dedicated split lands.
describe("WorkstationDetailCard editable configuration", () => {
  it("keeps configuration immediately after the workstation summary without requiring lower sections", () => {
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

    const summaryHeading = screen.getByRole("heading", {
      name: "Workstation summary",
    });
    const configurationHeading = screen.getByRole("heading", {
      name: "Configuration",
    });
    const activeWorkHeading = screen.getByRole("heading", {
      name: "Active work",
    });

    expectHeadingBefore(summaryHeading, configurationHeading);
    expectHeadingBefore(configurationHeading, activeWorkHeading);
    expect(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Request history" }),
    ).toBeNull();
  });

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

  it("stacks configuration fields vertically and keeps save and discard in the header only", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onSave = vi.fn();
    const onDiscard = vi.fn();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          ...buildReadyEditableConfigurationState(),
          isDirty: true,
          pendingFactoryDefinition: buildDetailCardEditableFactoryDocument(),
        }}
        headerAction={buildWorkstationHeaderActions({
          canDiscard: true,
          canSave: true,
          onDiscard,
          onSave,
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

    const fieldGroup = editableConfigurationSection().querySelector(
      CURRENT_SELECTION_FORM_FIELDS_SELECTOR,
    );
    expect(fieldGroup).not.toBeNull();
    expect(fieldGroup?.className).not.toMatch(/md:grid-cols-\d/);
    expect(fieldGroup?.className).not.toMatch(/xl:grid-cols-\d/);

    const headerActions = currentSelectionHeaderActionSection();
    const saveButtons = within(headerActions).getAllByRole("button", {
      name: "Save changes",
    });
    const discardButtons = within(headerActions).getAllByRole("button", {
      name: "Discard local changes",
    });
    expect(saveButtons).toHaveLength(1);
    expect(discardButtons).toHaveLength(1);

    fireEvent.click(saveButtons[0]);
    expect(onSave).toHaveBeenCalledTimes(1);

    fireEvent.click(discardButtons[0]);
    expect(onDiscard).toHaveBeenCalledTimes(1);

    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Save changes",
      }),
    ).toBeNull();
    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Discard local changes",
      }),
    ).toBeNull();
    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Reset to latest",
      }),
    ).toBeNull();
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
      screen.getByText(
        "Autocomplete is ready with 2 variables for 1 authored input.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Available variables")).toBeNull();
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

  it("renders runner selection and effective-runner help without a capability matrix", async () => {
    const user = userEvent.setup();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onRunnerChange = vi.fn();
    const readyState = buildReadyEditableConfigurationState();

    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          ...readyState,
          onRunnerChange,
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expandEditableConfiguration();

    const configuration = editableConfigurationSection();
    const runnerSelect = screen.getByRole("combobox", { name: "Runner" });

    expect(runnerSelect).toBeTruthy();
    expect(
      within(configuration).getByText(
        "Effective runner: Gemini (Workstation).",
      ),
    ).toBeTruthy();
    expect(
      within(configuration).queryByText("Runner capability support"),
    ).toBeNull();
    expect(within(configuration).queryByText("Supported")).toBeNull();
    expect(within(configuration).queryByText("Unsupported")).toBeNull();

    await selectLabeledComboboxOption(user, "Runner", "Codex");
    expect(onRunnerChange).toHaveBeenCalledWith("codex");

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          ...readyState,
          draft: {
            ...readyState.draft,
            runnerName: "codex",
          },
          onRunnerChange,
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(
      within(configuration).getByText("Effective runner: Codex (Workstation)."),
    ).toBeTruthy();
    expect(
      within(configuration).queryByText("Runner capability support"),
    ).toBeNull();
  });

  it("renders cron fields for CRON workstations with prefilled values", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onCronScheduleChange = vi.fn();
    const onCronTriggerAtStartChange = vi.fn();
    const onCronJitterChange = vi.fn();
    const onCronExpiryWindowChange = vi.fn();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          ...buildReadyEditableConfigurationState({
            behavior: "CRON",
            cron: {
              expiryWindow: "45s",
              jitter: "5s",
              schedule: "0 9 * * 1-5",
              triggerAtStart: true,
            },
          }),
          onCronExpiryWindowChange,
          onCronJitterChange,
          onCronScheduleChange,
          onCronTriggerAtStartChange,
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expandEditableConfiguration();

    expect(screen.getByLabelText("Cron schedule")).toBeTruthy();
    expect(screen.getByDisplayValue("0 9 * * 1-5")).toBeTruthy();
    expect(screen.getByLabelText("Cron trigger at start")).toBeTruthy();
    expect(
      (screen.getByLabelText("Cron trigger at start") as HTMLInputElement)
        .checked,
    ).toBe(true);
    expect(screen.getByLabelText("Cron jitter")).toBeTruthy();
    expect(screen.getByDisplayValue("5s")).toBeTruthy();
    expect(screen.getByLabelText("Cron expiry window")).toBeTruthy();
    expect(screen.getByDisplayValue("45s")).toBeTruthy();
    expect(
      screen.getByText(
        "Required five-field cron expression (for example */5 * * * *).",
      ),
    ).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Cron schedule"), {
      target: { value: "*/10 * * * *" },
    });
    fireEvent.change(screen.getByLabelText("Cron jitter"), {
      target: { value: "10s" },
    });
    fireEvent.change(screen.getByLabelText("Cron expiry window"), {
      target: { value: "2m" },
    });
    fireEvent.click(screen.getByLabelText("Cron trigger at start"));

    expect(onCronScheduleChange).toHaveBeenCalledWith("*/10 * * * *");
    expect(onCronJitterChange).toHaveBeenCalledWith("10s");
    expect(onCronExpiryWindowChange).toHaveBeenCalledWith("2m");
    expect(onCronTriggerAtStartChange).toHaveBeenCalledWith(false);
  });

  it("does not render cron fields for STANDARD, REPEATER, or POLLER workstations", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "STANDARD",
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expandEditableConfiguration();
    expect(screen.queryByLabelText("Cron schedule")).toBeNull();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "REPEATER",
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.queryByLabelText("Cron schedule")).toBeNull();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "POLLER",
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.queryByLabelText("Cron schedule")).toBeNull();
    expect(screen.queryByLabelText("Cron trigger at start")).toBeNull();
  });

  it("renders poller workstation summary annotation and behavior guidance from canonical behavior", () => {
    const snapshot = workstationKindParityDashboardSnapshot;
    const selectedNode =
      snapshot.topology.workstation_nodes_by_id["linear-poller"];

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "POLLER",
          workerName: "linear-poller",
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const kindSummary = screen.getByRole("img", {
      name: "Poller workstation",
    });
    expect(kindSummary.getAttribute("data-graph-semantic-icon")).toBe("poller");
    expect(screen.getByText("Poller")).toBeTruthy();

    expandEditableConfiguration();
    expect(
      screen.getByText(
        /Poller workstations supervise a long-lived ingress worker/i,
      ),
    ).toBeTruthy();
  });

  it("shows cron validation errors and overwrite hints in the editable form", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "CRON",
          overwriteFieldNames: ["cronSchedule", "cronJitter"],
          validationErrors: {
            cronJitter: 'jitter must be a non-negative duration, got "bad"',
            cronSchedule: "cron workstation requires non-empty 'schedule'",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expandEditableConfiguration();

    expect(
      screen.getByText("cron workstation requires non-empty 'schedule'"),
    ).toBeTruthy();
    expect(
      screen.getByText('jitter must be a non-negative duration, got "bad"'),
    ).toBeTruthy();
    expect(
      screen.getAllByText(
        "The running factory changed this field while you were editing. Reset to latest to discard the local draft value.",
      ),
    ).toHaveLength(2);
  });

  it("shows cron trigger-at-start save validation errors on the checkbox field", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "CRON",
          validationErrors: {
            cronTriggerAtStart: "trigger_at_start must be a boolean",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expandEditableConfiguration();

    expect(screen.getByText("trigger_at_start must be a boolean")).toBeTruthy();
  });

  it("localizes cron field labels in zh-CN for CRON workstations", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "CRON",
        })}
        locale="zh-CN"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const configurationSection = screen
      .getByRole("heading", { name: "配置" })
      .closest("section");
    if (!configurationSection) {
      throw new Error("expected localized editable configuration section");
    }

    fireEvent.click(
      within(configurationSection).getByRole("button", {
        name: "展开可编辑配置",
      }),
    );

    expect(screen.getByLabelText("Cron 调度")).toBeTruthy();
    expect(screen.getByLabelText("Cron 启动时触发")).toBeTruthy();
    expect(screen.getByLabelText("Cron 抖动")).toBeTruthy();
    expect(screen.getByLabelText("Cron 过期窗口")).toBeTruthy();
    expect(
      screen.getByText("必填的五字段 cron 表达式（例如 */5 * * * *）。"),
    ).toBeTruthy();
  });

  it("renders behavior, worker, and prompt controls in the editable form", async () => {
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
    expect(screen.getByRole("combobox", { name: "Worker" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Kind" })).toBeTruthy();
    expect(screen.queryByLabelText("Model")).toBeNull();
    expect(screen.queryByLabelText("Template")).toBeNull();

    const user = userEvent.setup();
    await selectLabeledComboboxOption(user, "Worker", "planner");
    fireEvent.change(screen.getByLabelText("Prompt"), {
      target: { value: "Updated prompt" },
    });

    expect(onWorkerChange).toHaveBeenCalledWith("planner");
    expect(onPromptChange).toHaveBeenCalledWith("Updated prompt");
  });

  it("marks server-changed fields and resets the draft from the header discard action", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const onResetToLatest = vi.fn();

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          ...buildReadyEditableConfigurationState({
            overwriteFieldNames: ["prompt", "worker"],
            prompt: "Keep this local prompt draft.",
          }),
          isDirty: true,
          onResetToLatest,
        }}
        headerAction={buildWorkstationHeaderActions({
          canDiscard: true,
          canSave: false,
          onDiscard: onResetToLatest,
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
      screen.getAllByText(
        "The running factory changed this field while you were editing. Reset to latest to discard the local draft value.",
      ),
    ).toHaveLength(2);

    fireEvent.click(
      within(currentSelectionHeaderActionSection()).getByRole("button", {
        name: "Discard local changes",
      }),
    );

    expect(onResetToLatest).toHaveBeenCalledTimes(1);
    expect(
      within(editableConfigurationSection()).queryByRole("button", {
        name: "Reset to latest",
      }),
    ).toBeNull();
  });

  it("identifies shared workers and keeps worker-owned fields out of the workstation form", () => {
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

    expect(screen.queryByText(/also used by/i)).toBeNull();
    expect(screen.queryByLabelText("Model")).toBeNull();
    expect(screen.queryByLabelText("Template")).toBeNull();
  });

  it("makes shared-worker scope explicit without exposing worker-owned settings", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          sharedWorkerWorkstationNames: ["Plan", "Code"],
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
      screen.getByText("Worker reviewer is also used by Plan, Code."),
    ).toBeTruthy();
    expect(
      screen.queryByText(
        /updates every workstation that references this worker/i,
      ),
    ).toBeNull();
    expect(screen.queryByLabelText("Model")).toBeNull();
    expect(screen.queryByLabelText("Template")).toBeNull();
  });

  it("keeps the shared-worker scope hint visible after reassigning to another shared worker", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          workerName: "planner",
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
      screen.getByText("Worker planner is also used by Plan, Code."),
    ).toBeTruthy();
    expect(
      screen.queryByText(
        /updates every workstation that references this worker/i,
      ),
    ).toBeNull();
  });

  it("localizes behavior options in zh-CN while keeping canonical option values", async () => {
    const user = userEvent.setup();
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState()}
        locale="zh-CN"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const configurationSection = screen
      .getByRole("heading", { name: "配置" })
      .closest("section");
    if (!configurationSection) {
      throw new Error("expected localized editable configuration section");
    }

    fireEvent.click(
      within(configurationSection).getByRole("button", {
        name: "展开可编辑配置",
      }),
    );

    const kindSelect = screen.getByRole("combobox", { name: "类型" });
    await user.click(kindSelect);
    const listbox = await screen.findByRole("listbox");

    expect(
      within(listbox)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["标准", "重复器", "轮询器"]);
  });

  it("renders localized workstation type from editable configuration when ready", () => {
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

    expect(screen.getByText("Workstation type")).toBeTruthy();
    expect(screen.getByText("Model workstation (legacy)")).toBeTruthy();
  });

  it("renders localized workstation type in zh-CN when editable configuration is ready", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState()}
        locale="zh-CN"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getByText("工作站类型")).toBeTruthy();
    expect(screen.getByText("模型工作站（旧版）")).toBeTruthy();
  });

  it("shows workstation type loading and unavailable copy for editable configuration states", () => {
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

    expect(screen.getByText("Loading workstation type...")).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          errorMessage: "Factory definition unavailable.",
          status: "error",
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getByText("Workstation type unavailable")).toBeTruthy();
  });

  it("renders localized scheduling kind from editable draft behavior in the workstation summary", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = {
      ...snapshot.topology.workstation_nodes_by_id.review,
      workstation_kind: "future-kind",
    };

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          behavior: "REPEATER",
        })}
        locale="zh-CN"
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getByRole("heading", { name: "工作站摘要" })).toBeTruthy();
    expect(screen.getByText("重复器")).toBeTruthy();
  });

  it("shows workstation kind loading and unavailable copy for editable configuration states", () => {
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

    expect(screen.getByText("Loading workstation kind...")).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          errorMessage: "Factory definition unavailable.",
          status: "error",
        }}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(screen.getByText("Workstation kind unavailable")).toBeTruthy();
  });

  it("shows inline Monaco guidance from the current workstation contract", () => {
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

    expandEditableConfiguration();

    expect(
      screen.getByText(
        "Autocomplete is ready with 2 variables for 1 authored input.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByText(
        "Suggestions appear only while typing inside {{ ... }}.",
      ),
    ).toBeNull();
    expect(screen.queryByText("Available variables")).toBeNull();
    expect(promptVariableHelpToggle()).toBeTruthy();
    expect(promptVariableHelpToggle().getAttribute("aria-expanded")).toBe(
      "false",
    );

    fireEvent.click(promptVariableHelpToggle());

    expect(
      screen.queryByText(
        "Suggestions appear only while typing inside {{ ... }}.",
      ),
    ).toBeNull();
    expect(
      screen.queryByText("Type inside {{ ... }} for suggestions."),
    ).toBeNull();
    expect(screen.getByText("Available variables")).toBeTruthy();
    expect(screen.getByText(".WorkID")).toBeTruthy();
    expect(screen.getByText("{{ .WorkID }}")).toBeTruthy();
    expect(screen.getByText("The current work item identifier.")).toBeTruthy();
    expect(screen.getByText("Unavailable access")).toBeTruthy();
    expect(screen.getByText(".Inputs[1].Payload")).toBeTruthy();
    expect(
      screen.getByText("Only input 0 is available for this workstation."),
    ).toBeTruthy();
    expect(
      within(editableConfigurationSection())
        .getByRole("button", {
          name: "Close prompt variable help",
        })
        .getAttribute("aria-expanded"),
    ).toBe("true");
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
            prompt: "See prompt diagnostics below.",
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
    fireEvent.click(promptVariableHelpToggle());

    expect(screen.getByText("Prompt diagnostics")).toBeTruthy();
    expect(
      screen.getAllByText("Fix highlighted issues before saving."),
    ).toHaveLength(1);
    expect(
      screen.queryByText(
        "Save stays disabled until the prompt validates cleanly for this workstation context.",
      ),
    ).toBeNull();
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

  it("shows line-based syntax diagnostics separately from variable-access diagnostics", () => {
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
              message: "line 1: unexpected EOF in if block",
              sourceText: "{{ if .WorkID }}",
              startOffset: 5,
            },
          ],
          validationErrors: {
            prompt: "See prompt diagnostics below.",
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
    fireEvent.click(promptVariableHelpToggle());

    expect(screen.getByText("line 1: unexpected EOF in if block")).toBeTruthy();
    expect(
      screen.queryByText("Template syntax: unexpected EOF in if block"),
    ).toBeNull();
    expect(
      screen.queryByText("Variable access: unexpected EOF in if block"),
    ).toBeNull();
  });

  it(
    "keeps the squiggle aligned for runtime-generated diagnostics beyond column one",
    () => {
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
              prompt: "See prompt diagnostics below.",
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

      const editor = editableConfigurationSection().querySelector(
        "[data-monaco-editor='workstation-prompt']",
      );
      expect(editor?.getAttribute("data-monaco-marker-count")).toBe("1");
      expect(editor?.getAttribute("data-monaco-marker-messages")).toContain(
        "Template execution would fail: value has type int; should be string.",
      );
    },
    editableConfigurationCoverageTimeoutMs,
  );

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
      screen.queryByText("Validating prompt variables for the current draft."),
    ).toBeNull();

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
    fireEvent.click(promptVariableHelpToggle());

    expect(
      screen.getByText(
        "Prompt validation unavailable. Prompt validation API unavailable.",
      ),
    ).toBeTruthy();
  });

  it("resizes the prompt editor horizontally and keeps help and diagnostics below the editor", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    const { container } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          promptHelpState: {
            contract: {
              availableVariables: [
                {
                  category: "ROOT",
                  description: "Work item id.",
                  example: "{{ .WorkID }}",
                  path: ".WorkID",
                },
              ],
              inputCount: 1,
              unavailableAccessPatterns: [],
            },
            status: "ready",
          },
          promptDiagnostics: [
            {
              kind: "INVALID_VARIABLE",
              message: "Prompt root is invalid.",
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

    const section = editableConfigurationSection();
    const resizable = section.querySelector(
      "[data-prompt-editor-resizable='true']",
    ) as HTMLElement;
    const handle = within(section).getByRole("slider", {
      name: "Resize prompt editor height",
    });
    const promptEditor = screen.getByLabelText("Prompt");

    Object.defineProperty(resizable, "offsetHeight", {
      configurable: true,
      value: 216,
    });

    fireEvent.pointerDown(handle, { button: 0, clientY: 120, pointerId: 11 });
    fireEvent.pointerMove(handle, { clientY: 220, pointerId: 11 });
    fireEvent.pointerUp(handle, { pointerId: 11 });

    expect(resizable.style.height).toBe("316px");
    expect(promptEditor.parentElement?.getAttribute("aria-describedby")).toBe(
      "editable-workstation-prompt-error editable-workstation-prompt-diagnostics",
    );
    expectHeadingBefore(resizable, promptVariableHelpToggle());
    fireEvent.click(promptVariableHelpToggle());
    const diagnosticsPanel = document.getElementById(
      "editable-workstation-prompt-diagnostics",
    );
    expectHeadingBefore(resizable, diagnosticsPanel as HTMLElement);
    expect(
      container.querySelector("[data-prompt-editor-resizable='true']"),
    ).toBeTruthy();
  });

  it("links prompt editor accessibility metadata to validation and diagnostic feedback", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          prompt: "Use {{ .Prompt }} now.",
          promptDiagnostics: [
            {
              kind: "INVALID_VARIABLE",
              message: "Prompt root is invalid.",
              sourceText: "{{ .Prompt }}",
            },
          ],
          validationErrors: {
            prompt: "See prompt diagnostics below.",
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

    const promptEditor = screen.getByLabelText("Prompt");
    const promptEditorWrapper = promptEditor.parentElement;

    expect(promptEditorWrapper?.getAttribute("aria-invalid")).toBe("true");
    expect(promptEditorWrapper?.getAttribute("aria-describedby")).toBe(
      "editable-workstation-prompt-error editable-workstation-prompt-diagnostics",
    );
    expect(
      document.getElementById("editable-workstation-prompt-error")?.textContent,
    ).toBe("See prompt diagnostics below.");
    expect(
      document.getElementById("editable-workstation-prompt-diagnostics"),
    ).toBeTruthy();
    expect(
      screen.getByText("Prompt diagnostics").closest(".invisible"),
    ).toBeTruthy();
    expect(
      editableConfigurationSection().querySelector(
        "[data-prompt-diagnostics-reserved='true']",
      )?.className,
    ).toContain("min-h-24");
  });

  it("keeps a reserved prompt diagnostics region mounted across validation transitions", () => {
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

    expandEditableConfiguration();

    const reservedRegion = () =>
      editableConfigurationSection().querySelector(
        "[data-prompt-diagnostics-reserved='true']",
      );

    expect(reservedRegion()?.className).toContain("min-h-24");
    expect(
      document.getElementById("editable-workstation-prompt-diagnostics"),
    ).toBeTruthy();

    rerender(
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
          promptValidationState: { status: "ready" },
          validationErrors: {
            prompt: "See prompt diagnostics below.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expect(reservedRegion()?.className).toContain("min-h-24");
    expect(
      document.getElementById("editable-workstation-prompt-diagnostics"),
    ).toBeTruthy();
    expect(
      editableConfigurationSection().querySelectorAll(
        "#editable-workstation-prompt-diagnostics",
      ),
    ).toHaveLength(1);
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
            prompt: "See prompt diagnostics below.",
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

    const editor = editableConfigurationSection().querySelector(
      "[data-monaco-editor='workstation-prompt']",
    );
    expect(editor?.getAttribute("data-monaco-marker-count")).toBe("2");
    expect(editor?.getAttribute("data-monaco-marker-ranges")).toContain(
      '"startColumn":5',
    );
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
            prompt: "See prompt diagnostics below.",
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

    const editor = editableConfigurationSection().querySelector(
      "[data-monaco-editor='workstation-prompt']",
    );
    expect(editor?.getAttribute("data-monaco-marker-ranges")).toContain(
      '"startColumn":4',
    );
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
            prompt: "See prompt diagnostics below.",
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

    const editor = editableConfigurationSection().querySelector(
      "[data-monaco-editor='workstation-prompt']",
    );
    expect(editor?.getAttribute("data-monaco-marker-ranges")).toContain(
      '"endColumn":7',
    );
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
            prompt: "See prompt diagnostics below.",
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

    const editor = editableConfigurationSection().querySelector(
      "[data-monaco-editor='workstation-prompt']",
    );
    expect(editor?.getAttribute("data-monaco-marker-count")).toBe("2");
  });

  it("collapses ready prompt variable help by default and preserves list content when expanded", async () => {
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

    expandEditableConfiguration();

    const toggle = promptVariableHelpToggle();
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(toggle.getAttribute("aria-controls")).toBeTruthy();
    expect(screen.queryByText("Available variables")).toBeNull();
    expect(screen.queryByText("Unavailable access")).toBeNull();

    toggle.focus();
    await user.keyboard("{Enter}");

    expect(
      within(editableConfigurationSection())
        .getByRole("button", {
          name: "Close prompt variable help",
        })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(screen.getByText("Available variables")).toBeTruthy();
    expect(screen.getByText(".WorkID")).toBeTruthy();
    expect(screen.getByText("Unavailable access")).toBeTruthy();
    expect(screen.getByText(".Inputs[1].Payload")).toBeTruthy();

    await user.keyboard(" ");

    expect(promptVariableHelpToggle().getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByText("Available variables")).toBeNull();
  });

  it("keeps prompt variable help collapsed when diagnostics appear after the initial render", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState()}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expandEditableConfiguration();
    expect(promptVariableHelpToggle().getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByText("Available variables")).toBeNull();

    rerender(
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
            prompt: "See prompt diagnostics below.",
          },
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const toggle = promptVariableHelpToggle();
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("Available variables")).toBeNull();
    expect(screen.queryByText("Unavailable access")).toBeNull();

    const editor = editableConfigurationSection().querySelector(
      "[data-monaco-editor='workstation-prompt']",
    );
    expect(editor?.getAttribute("data-monaco-marker-count")).toBe("1");
    expect(editor?.closest(".border-af-danger-border")?.className).toContain(
      "border-af-danger-border",
    );
  });

  it("resets prompt variable help disclosure when the selected workstation changes", () => {
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

    expandEditableConfiguration();
    fireEvent.click(promptVariableHelpToggle());
    expect(screen.getByText("Available variables")).toBeTruthy();

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          initialValuesWorkstationName: "Plan",
          prompt: "Plan the next change.",
          workerName: "planner",
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={planNode}
      />,
    );

    expandEditableConfiguration();
    expect(promptVariableHelpToggle().getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByText("Available variables")).toBeNull();
  });

  it("keeps loading, empty, and error prompt help states outside the disclosure", () => {
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

    expect(
      screen.queryByRole("button", { name: "Open prompt variable help" }),
    ).toBeNull();

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
    expect(
      screen.getByText(
        "Loading the current factory definition for this workstation.",
      ).className,
    ).toContain("text-on-surface-variant");

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
    expect(
      screen.getByText(
        "Editable configuration unavailable. The current factory API rejected the request.",
      ).className,
    ).toContain("text-on-error-container");

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
    expect(
      screen.getByText(
        "This running factory definition does not expose editable worker and prompt values for the selected workstation.",
      ).className,
    ).toContain("text-on-surface-variant");
  });

  it("does not render inline save outcome copy in the configuration section", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const { rerender } = render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState()}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        saveState={{ status: "success" }}
        selectedNode={selectedNode}
      />,
    );

    fireEvent.click(
      within(editableConfigurationSection()).getByRole("button", {
        name: "Expand editable configuration",
      }),
    );

    expectNoInlineSaveOutcomesIn(editableConfigurationSection());

    rerender(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState()}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        saveState={{
          errorMessage: "The current factory rejected the workstation update.",
          status: "error",
        }}
        selectedNode={selectedNode}
      />,
    );

    expectNoInlineSaveOutcomesIn(editableConfigurationSection());
  });

  it("uses semantic panels for autocomplete and diagnostics feedback", () => {
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
            prompt: "See prompt diagnostics below.",
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
      screen
        .getByText(
          "Autocomplete is ready with 2 variables for 1 authored input.",
        )
        .closest(".border-outline")?.className,
    ).toContain("border-outline");
    fireEvent.click(promptVariableHelpToggle());
    expect(
      screen.getByText("Prompt diagnostics").closest("[role='alert']")
        ?.className,
    ).toContain("border-af-danger-border");
    expect(screen.getByText(".Inputs[1]").className).toContain(
      "text-on-surface-variant",
    );
  });

  it("hides worker, runner, prompt, and kind fields for LOGICAL_MOVE configuration", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = {
      ...snapshot.topology.workstation_nodes_by_id.review,
      workstation_kind: "LOGICAL_MOVE",
    };

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          workerName: "removed-worker",
          workstationType: "LOGICAL_MOVE",
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

    const configuration = editableConfigurationSection();
    expect(within(configuration).queryByLabelText("Worker")).toBeNull();
    expect(within(configuration).queryByLabelText("Kind")).toBeNull();
    expect(within(configuration).queryByLabelText("Runner")).toBeNull();
    expect(within(configuration).queryByLabelText("Prompt")).toBeNull();
    expect(
      within(configuration).queryByText("Worker selection unavailable."),
    ).toBeNull();
    expect(
      configuration.querySelector(CURRENT_SELECTION_FORM_FIELDS_SELECTOR),
    ).toBeNull();
  });

  it("does not surface worker-unavailable copy for LOGICAL_MOVE with a legacy missing worker", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = {
      ...snapshot.topology.workstation_nodes_by_id.review,
      workstation_kind: "LOGICAL_MOVE",
    };

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          workerName: "missing-worker",
          workerOptionsState: {
            message:
              "The selected workstation references a worker that is no longer available in the current factory definition. Reload current selection and choose another worker.",
            status: "error",
          },
          workstationType: "LOGICAL_MOVE",
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

    expect(screen.queryByText(/Worker selection unavailable/i)).toBeNull();
    expect(
      screen.queryByText(
        /no longer available in the current factory definition/i,
      ),
    ).toBeNull();
  });

  it("hides worker, runner, and kind summary tiles for LOGICAL_MOVE while keeping workstation type", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = {
      ...snapshot.topology.workstation_nodes_by_id.review,
      worker_type: "MODEL_WORKER",
      workstation_kind: "LOGICAL_MOVE",
    };

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          workerName: "removed-worker",
          workstationType: "LOGICAL_MOVE",
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    const summarySection = screen
      .getByRole("heading", { name: "Workstation summary" })
      .closest("section");
    const resolvedSummarySection = requireValue(
      summarySection,
      "expected workstation summary section",
    );

    expect(
      within(resolvedSummarySection).getByText("Logical move"),
    ).toBeTruthy();
    expect(
      within(resolvedSummarySection).queryByText("Worker type"),
    ).toBeNull();
    expect(
      within(resolvedSummarySection).queryByText("Selected runner"),
    ).toBeNull();
    expect(within(resolvedSummarySection).queryByText("Kind")).toBeNull();
    expect(
      within(resolvedSummarySection).queryByText("MODEL_WORKER"),
    ).toBeNull();
  });

  it("still renders worker-backed summary tiles for model workstations", () => {
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

    const summarySection = screen
      .getByRole("heading", { name: "Workstation summary" })
      .closest("section");
    const resolvedSummarySection = requireValue(
      summarySection,
      "expected workstation summary section",
    );

    expect(
      within(resolvedSummarySection).getByText("Worker type"),
    ).toBeTruthy();
    expect(
      within(resolvedSummarySection).getByText("Selected runner"),
    ).toBeTruthy();
    expect(within(resolvedSummarySection).getByText("Kind")).toBeTruthy();
    expect(
      within(resolvedSummarySection).getByText("Model workstation (legacy)"),
    ).toBeTruthy();
  });

  it("still renders worker-backed configuration fields for model workstations", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={buildReadyEditableConfigurationState({
          workstationType: "MODEL_WORKSTATION",
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

    const configuration = editableConfigurationSection();
    expect(within(configuration).getByLabelText("Worker")).toBeTruthy();
    expect(within(configuration).getByLabelText("Kind")).toBeTruthy();
    expect(within(configuration).getByLabelText("Runner")).toBeTruthy();
    expect(within(configuration).getByLabelText("Prompt")).toBeTruthy();
  });

  it("omits global unsaved helper paragraphs for dirty ready-state workstation drafts", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        editableConfigurationState={{
          ...buildReadyEditableConfigurationState({
            prompt: "Updated prompt for review.",
          }),
          isDirty: true,
          pendingFactoryDefinition: buildDetailCardEditableFactoryDocument(),
        }}
        headerAction={buildWorkstationHeaderActions({
          canDiscard: true,
          canSave: true,
        })}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    expandEditableConfiguration();

    expect(
      screen.queryByText("You have unsaved changes for this workstation."),
    ).toBeNull();
    expect(
      screen.queryByText(
        "Changes stay local to this edit session until you save the running factory.",
      ),
    ).toBeNull();
    expect(
      screen.getAllByRole("button", { name: "Save changes" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: "Discard local changes" }),
    ).toBeTruthy();
  });
});
