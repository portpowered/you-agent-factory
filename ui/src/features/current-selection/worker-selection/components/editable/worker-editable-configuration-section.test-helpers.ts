import { vi } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../../../api/factory-definition/api";
import type { EditableWorkerConfigurationState } from "../../lib/detail-card-types";
import { getWorkerDetailMessages } from "../../messages/worker-detail";

const EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS = {
  authSecretRef: "",
  linearClaimAssigneeField: "",
  linearMappingState: "",
  linearMappingWorkType: "",
  linearPollInterval: "",
  linearStateIdsText: "",
  linearTeamIdsText: "",
};

const EMPTY_HOSTED_LINEAR_EDITABLE_VALUES = {
  authSecretRef: null,
  linearClaimAssigneeField: null,
  linearClaimPresent: false,
  linearMappingState: null,
  linearMappingWorkType: null,
  linearPollInterval: null,
  linearStateIds: [] as string[],
  linearTeamIds: [] as string[],
};

export const workerEditableConfigurationSectionMessages =
  getWorkerDetailMessages();

export function buildReadyWorkerEditableConfigurationState(
  workstationNames: string[],
): Extract<EditableWorkerConfigurationState, { status: "ready" }> {
  const savedFactoryDefinition = {
    name: "Current Factory",
    workers: [],
    workstations: [],
  } as CanonicalFactoryDefinition;

  return {
    baseVersion: {
      logical: "1",
      physical: "2026-06-08T00:00:00Z",
    },
    canSave: true,
    draft: {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CODEX",
      name: "reviewer",
      provider: null,
      skipPermissions: false,
      stopToken: "",
      timeoutAmount: "",
      timeoutUnit: "m",
      type: "MODEL_WORKER",
      ...EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS,
    },
    hasValidationErrors: false,
    initialValues: {
      args: [],
      body: null,
      command: null,
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CODEX",
      provider: null,
      skipPermissions: null,
      stopToken: null,
      timeout: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames,
      ...EMPTY_HOSTED_LINEAR_EDITABLE_VALUES,
    },
    isDirty: true,
    onArgsTextChange: vi.fn(),
    onAuthSecretRefChange: vi.fn(),
    onBodyChange: vi.fn(),
    onCommandChange: vi.fn(),
    onExecutorProviderChange: vi.fn(),
    onLinearClaimAssigneeFieldChange: vi.fn(),
    onLinearMappingStateChange: vi.fn(),
    onLinearMappingWorkTypeChange: vi.fn(),
    onLinearPollIntervalChange: vi.fn(),
    onLinearStateIdsTextChange: vi.fn(),
    onLinearTeamIdsTextChange: vi.fn(),
    onModelChange: vi.fn(),
    onModelLocalityChange: vi.fn(),
    onModelProviderChange: vi.fn(),
    onNameChange: vi.fn(),
    onProviderChange: vi.fn(),
    onSkipPermissionsChange: vi.fn(),
    onStopTokenChange: vi.fn(),
    onTimeoutAmountChange: vi.fn(),
    onTimeoutUnitChange: vi.fn(),
    markChangesSaved: vi.fn(),
    onResetToLatest: vi.fn(),
    onTypeChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: null,
    savedFactoryDefinition,
    status: "ready",
    validationErrors: {},
  };
}
