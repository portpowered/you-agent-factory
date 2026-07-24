import { fireEvent, screen } from "@testing-library/react";
import { vi } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../../../api/factory-definition/api";
import { resolveEditableWorkstationTypeConversionOptions } from "../../../../current-factory-definition/public";
import type { EditableWorkstationConfigurationState } from "../../lib/keys/detail-card-types";
import { getWorkstationDetailMessages } from "../../messages/workstation-detail";

type EditableConfigurationSectionReadyState = Extract<
  EditableWorkstationConfigurationState,
  { status: "ready" }
>;

export const editableConfigurationSectionMessages =
  getWorkstationDetailMessages();

export function expandEditableConfigurationSection() {
  fireEvent.click(
    screen.getByRole("button", { name: "Expand editable configuration" }),
  );
}

function buildEditableConfigurationSectionDraftDefaults(
  overrides?: Partial<{
    behavior?: "STANDARD" | "REPEATER" | "POLLER" | "CRON";
    cron?: {
      expiryWindow: string;
      jitter: string;
      schedule: string;
      triggerAtStart: boolean;
    } | null;
    guards: Array<{
      type: "VISIT_COUNT";
      workstation: string;
      maxVisits: number;
    }>;
    name?: string;
    operation?: string;
    operationBindings?: Array<{
      slot: string;
      configText?: string;
      defaultContentText?: string;
      selector: {
        label: string;
        role: string;
        slot: string;
        type: string;
      };
    }>;
    prompt?: string;
    runnerName?: string;
    workerName?: string;
  }>,
  workstationType:
    | "AGENT_RUN"
    | "INFERENCE_RUN"
    | "MODEL_WORKSTATION"
    | "MODEL_INVOKE"
    | "LOGICAL_MOVE" = "AGENT_RUN",
) {
  const behavior = overrides?.behavior ?? ("STANDARD" as const);
  const cron =
    overrides?.cron !== undefined
      ? overrides.cron
      : behavior === "CRON"
        ? {
            expiryWindow: "30m",
            jitter: "5s",
            schedule: "0 9 * * *",
            triggerAtStart: true,
          }
        : null;

  return {
    behavior,
    cron,
    guards: overrides?.guards ?? [],
    inputs: [],
    name: overrides?.name ?? "Review",
    operation: overrides?.operation ?? "TTS",
    operationBindings: overrides?.operationBindings ?? [
      {
        slot: "text",
        configText: "",
        defaultContentText: "",
        selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
      },
    ],
    prompt: overrides?.prompt ?? "Review prompt",
    runnerName: overrides?.runnerName ?? ("gemini" as const),
    workerName: overrides?.workerName ?? "reviewer",
    workstationType,
  };
}

function buildEditableConfigurationSectionInitialValues(
  overrides?: Partial<{
    sharedWorkerWorkstationNamesByWorkerName: Record<string, string[]>;
  }>,
  workstationType:
    | "AGENT_RUN"
    | "INFERENCE_RUN"
    | "MODEL_WORKSTATION"
    | "MODEL_INVOKE"
    | "LOGICAL_MOVE" = "AGENT_RUN",
) {
  return {
    behavior: "STANDARD" as const,
    behaviorOptions: ["STANDARD", "REPEATER", "POLLER"] as const,
    cron: null,
    effectiveRunnerName: "gemini",
    factoryRunnerName: "codex",
    guards: [],
    inputs: [],
    modelInvokeWorkerOptions: ["tts-worker"],
    modelOperationsByWorkerName: {
      "tts-worker": [
        {
          name: "TTS",
          inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
          outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
        },
      ],
    },
    operation: "TTS",
    operationBindings: [
      {
        slot: "text",
        configText: "",
        defaultContentText: "",
        selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
      },
    ],
    prompt: "Review prompt",
    resolvedRunnerSelection: {
      runnerId: "gemini",
      source: "workstation" as const,
    },
    runnerName: "gemini",
    runnerOptions: ["codex", "gemini"],
    runnerSelectionSource: "workstation" as const,
    sharedWorkerWorkstationNames: [],
    sharedWorkerWorkstationNamesByWorkerName:
      overrides?.sharedWorkerWorkstationNamesByWorkerName ?? {},
    workerModelProvider: null,
    workerName: "reviewer",
    workerOptions: ["reviewer", "planner"],
    workerTypeByName: {
      planner: "MODEL_WORKER" as const,
      reviewer: "MODEL_WORKER" as const,
    },
    workstationName: "Review",
    workstationOptions: ["Plan", "Review"],
    workstationType,
    workstationTypeOptions:
      resolveEditableWorkstationTypeConversionOptions(workstationType),
  };
}

export function buildEditableConfigurationSectionReadyState(
  overrides?: Partial<{
    draft: {
      behavior?: "STANDARD" | "REPEATER" | "POLLER" | "CRON";
      cron?: {
        expiryWindow: string;
        jitter: string;
        schedule: string;
        triggerAtStart: boolean;
      } | null;
      guards: Array<{
        type: "VISIT_COUNT";
        workstation: string;
        maxVisits: number;
      }>;
      name?: string;
      operation?: string;
      operationBindings?: Array<{
        slot: string;
        configText?: string;
        defaultContentText?: string;
        selector: {
          label: string;
          role: string;
          slot: string;
          type: string;
        };
      }>;
      prompt?: string;
      runnerName?: string;
      workerName?: string;
    };
    operationOptionsState:
      | {
          operations: Array<{
            name: string;
            inputs?: Array<{
              name: string;
              contentTypes: string[];
              required?: boolean;
            }>;
            outputs?: Array<{ name: string; contentTypes: string[] }>;
          }>;
          options: string[];
          status: "ready";
        }
      | { message: string; status: "empty" | "error" };
    hasValidationErrors: boolean;
    initialValues: Partial<{
      sharedWorkerWorkstationNamesByWorkerName: Record<string, string[]>;
    }>;
    isDirty: boolean;
    pendingFactoryDefinition: CanonicalFactoryDefinition | null;
    promptDiagnostics: Array<{ message: string; severity: "error" }>;
    validationErrors: Record<string, string | undefined>;
    workerOptionsState:
      | { status: "ready"; options: string[] }
      | { message: string; status: "empty" | "error" };
    overwriteFieldNames: Array<
      "name" | "worker" | "prompt" | "behavior" | "runner"
    >;
    workstationType:
      | "AGENT_RUN"
      | "INFERENCE_RUN"
      | "MODEL_WORKSTATION"
      | "MODEL_INVOKE"
      | "LOGICAL_MOVE";
  }>,
): EditableConfigurationSectionReadyState {
  const workstationType = overrides?.workstationType ?? "AGENT_RUN";

  return {
    draft: buildEditableConfigurationSectionDraftDefaults(
      overrides?.draft,
      workstationType,
    ),
    hasValidationErrors: overrides?.hasValidationErrors ?? false,
    initialValues: buildEditableConfigurationSectionInitialValues(
      overrides?.initialValues,
      workstationType,
    ),
    isDirty: overrides?.isDirty ?? false,
    markChangesSaved: vi.fn(),
    baseVersion: { logical: "1", physical: "2026-06-01T00:00:00Z" },
    onBehaviorChange: vi.fn(),
    onCronExpiryWindowChange: vi.fn(),
    onCronJitterChange: vi.fn(),
    onCronScheduleChange: vi.fn(),
    onCronTriggerAtStartChange: vi.fn(),
    onGuardsChange: vi.fn(),
    onInputsChange: vi.fn(),
    onNameChange: vi.fn(),
    onOperationBindingsChange: vi.fn(),
    onOperationChange: vi.fn(),
    onPromptChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onRunnerChange: vi.fn(),
    onWorkstationTypeChange: vi.fn(),
    onWorkerChange: vi.fn(),
    overwriteFieldNames: overrides?.overwriteFieldNames ?? [],
    pendingFactoryDefinition:
      overrides?.pendingFactoryDefinition === undefined
        ? ({ workstations: [] } as unknown as CanonicalFactoryDefinition)
        : overrides.pendingFactoryDefinition,
    promptDiagnostics: overrides?.promptDiagnostics ?? [],
    promptHelpState: { status: "empty" as const, message: "" },
    promptValidationState: {
      diagnostics: [],
      result: { diagnostics: [], valid: true },
      status: "ready" as const,
    },
    status: "ready" as const,
    validationErrors: overrides?.validationErrors ?? {},
    operationOptionsState: overrides?.operationOptionsState ?? {
      operations: [
        {
          name: "TTS",
          inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
          outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
        },
      ],
      options: ["TTS"],
      status: "ready" as const,
    },
    workerOptionsState: overrides?.workerOptionsState ?? {
      options: ["reviewer", "planner"],
      status: "ready" as const,
    },
    workstationOptionsState: {
      options: ["Plan", "Review"],
      status: "ready" as const,
    },
  } as unknown as EditableConfigurationSectionReadyState;
}
