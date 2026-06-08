import { vi } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../../api/factory-definition/api";
import {
  EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS,
  EMPTY_HOSTED_LINEAR_EDITABLE_VALUES,
} from "../../../current-factory-definition/lib/worker-editable-values";
import type { EditableWorkerConfigurationState } from "../lib/detail-card-types";
import { getWorkerDetailMessages } from "../messages/worker-detail";

export const workerEditableConfigurationSectionMessages =
  getWorkerDetailMessages();

export function buildReadyWorkerEditableConfigurationState(
  workstationNames: string[],
): Extract<EditableWorkerConfigurationState, { status: "ready" }> {
  return {
    canSave: true,
    draft: {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
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
      modelProvider: "CURSOR",
      name: "reviewer",
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
    pendingFactoryDefinition: {} as CanonicalFactoryDefinition,
    status: "ready",
    validationErrors: {},
  };
}
