import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../../../../styles.css";
import { expectStyledCheckboxInStory } from "../../../../../testing/checkbox-story-helpers";
import type { EditableWorkerSaveValidationErrors } from "../../lib/detail-card-types";
import { getWorkerDetailMessages } from "../../messages/worker-detail";
import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";

const messages = getWorkerDetailMessages();

function buildStoryReadyWorkerState(
  skipPermissions: boolean,
  validationErrors: EditableWorkerSaveValidationErrors,
) {
  const hasValidationErrors = Object.keys(validationErrors).length > 0;

  return {
    baseVersion: {
      logical: "1",
      physical: "2026-06-08T00:00:00Z",
    },
    canSave: !hasValidationErrors,
    draft: {
      argsText: "",
      authSecretRef: "",
      body: "",
      command: "",
      executorProvider: null,
      linearClaimAssigneeField: "",
      linearMappingState: "",
      linearMappingWorkType: "",
      linearPollInterval: "",
      linearStateIdsText: "",
      linearTeamIdsText: "",
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CODEX",
      name: "reviewer",
      provider: null,
      skipPermissions,
      stopToken: "",
      timeoutAmount: "",
      timeoutUnit: "m",
      type: "MODEL_WORKER" as const,
    },
    hasValidationErrors,
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
      type: "MODEL_WORKER" as const,
      workerName: "reviewer",
      workstationNames: ["Review"],
      authSecretRef: null,
      linearClaimAssigneeField: null,
      linearClaimPresent: false,
      linearMappingState: null,
      linearMappingWorkType: null,
      linearPollInterval: null,
      linearStateIds: [],
      linearTeamIds: [],
    },
    isDirty: true,
    onArgsTextChange: () => undefined,
    onAuthSecretRefChange: () => undefined,
    onBodyChange: () => undefined,
    onCommandChange: () => undefined,
    onExecutorProviderChange: () => undefined,
    onLinearClaimAssigneeFieldChange: () => undefined,
    onLinearMappingStateChange: () => undefined,
    onLinearMappingWorkTypeChange: () => undefined,
    onLinearPollIntervalChange: () => undefined,
    onLinearStateIdsTextChange: () => undefined,
    onLinearTeamIdsTextChange: () => undefined,
    onModelChange: () => undefined,
    onModelLocalityChange: () => undefined,
    onModelProviderChange: () => undefined,
    onNameChange: () => undefined,
    onProviderChange: () => undefined,
    onSkipPermissionsChange: () => undefined,
    onStopTokenChange: () => undefined,
    onTimeoutAmountChange: () => undefined,
    onTimeoutUnitChange: () => undefined,
    markChangesSaved: () => undefined,
    onResetToLatest: () => undefined,
    onTypeChange: () => undefined,
    overwriteFieldNames: [],
    pendingFactoryDefinition: null,
    savedFactoryDefinition: {
      name: "Current Factory",
      workers: [],
      workstations: [],
    },
    status: "ready" as const,
    validationErrors,
  };
}

function WorkerSkipPermissionsVerificationStory({
  validationErrors = {},
}: {
  validationErrors?: EditableWorkerSaveValidationErrors;
}) {
  const [skipPermissions, setSkipPermissions] = useState(false);
  const state = buildStoryReadyWorkerState(skipPermissions, validationErrors);

  return (
    <WorkerEditableConfigurationSection
      messages={messages}
      state={{
        ...state,
        onSkipPermissionsChange: setSkipPermissions,
      }}
      workerName="reviewer"
    />
  );
}

export default {
  title: "you-agent-factory/Checkbox Consistency/Current Selection",
  tags: ["test"],
};

export const WorkerSkipPermissions = {
  render: () => <WorkerSkipPermissionsVerificationStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const checkbox = await canvas.findByRole("checkbox", {
      name: messages.skipPermissionsFieldLabel,
    });

    expectStyledCheckboxInStory(checkbox);
    expect(checkbox).not.toBeChecked();
    await expect(
      canvas.getByText(messages.skipPermissionsFieldHelp),
    ).toBeVisible();

    await userEvent.click(canvas.getByText(messages.skipPermissionsFieldLabel));
    expect(checkbox).toBeChecked();

    checkbox.focus();
    await userEvent.keyboard(" ");
    expect(checkbox).not.toBeChecked();
  },
};

export const WorkerSkipPermissionsInvalid = {
  render: () => (
    <WorkerSkipPermissionsVerificationStory
      validationErrors={{
        skipPermissions: "skipPermissions must be a boolean.",
      }}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const checkbox = await canvas.findByRole("checkbox", {
      name: messages.skipPermissionsFieldLabel,
    });

    expectStyledCheckboxInStory(checkbox);
    expect(checkbox).toHaveAttribute("aria-invalid", "true");
    await expect(
      canvas.getByText("skipPermissions must be a boolean."),
    ).toBeVisible();
  },
};
