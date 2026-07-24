import { OptionalEnumSelect } from "@you-agent-factory/components/forms";
import { Text } from "../../../../../components/ui";
import { resolveRunnerSelection } from "../../../../current-factory-definition/lib/runner-selection";
import {
  getRunnerDisplayName,
  getRunnerMetadata,
} from "../../editing/runner-metadata";
import type { WorkstationDetailCardProps } from "../../lib/keys/detail-card-types";
import {
  type ApiRunnerID,
  isOpenApiRunnerID,
} from "../../messages/runner-openapi-enums";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";

type ReadyEditableConfigurationState = Extract<
  NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
  { status: "ready" }
>;

export function EditableConfigurationRunnerField({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
}) {
  const resolvedSelection = resolveEditableRunnerSelection(state);
  const inheritedRunnerLabel = state.initialValues.factoryRunnerName
    ? messages.runnerInheritanceFactoryLabel(
        getRunnerDisplayName(state.initialValues.factoryRunnerName) ??
          state.initialValues.factoryRunnerName,
      )
    : messages.runnerInheritanceFactoryMissingLabel;

  const runnerName =
    getRunnerDisplayName(resolvedSelection.runnerId) ??
    resolvedSelection.runnerId;
  const sourceLabel = messages.localizeRunnerSelectionSource(
    resolvedSelection.source,
  );

  return (
    <div className="grid gap-2">
      <OptionalEnumSelect
        aria-describedby={
          state.validationErrors.runnerName
            ? "editable-workstation-runner-error"
            : undefined
        }
        aria-invalid={state.validationErrors.runnerName ? "true" : undefined}
        aria-label={messages.runnerFieldLabel}
        emptyOptionLabel={inheritedRunnerLabel}
        id="editable-workstation-runner"
        onValueChange={(nextValue) =>
          state.onRunnerChange(
            nextValue === null ? null : (nextValue as ApiRunnerID),
          )
        }
        options={state.initialValues.runnerOptions.map((runnerID) => ({
          label: getRunnerDisplayName(runnerID) ?? runnerID,
          value: runnerID,
        }))}
        value={state.draft.runnerName}
      />
      <Text className="m-0 text-on-surface-subtle" variant="supporting">
        {messages.runnerFieldHelp(runnerName, sourceLabel)}
      </Text>
    </div>
  );
}

export function resolveWorkerBackedWorkstationSummaryRunnerValue(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  messages: ReturnType<typeof getWorkstationDetailMessages>,
): string {
  if (!state || state.status === "loading") {
    return messages.runnerLoadingValue;
  }

  if (state.status === "error" || state.status === "empty") {
    return messages.unavailableRunnerValue;
  }

  if (
    state.draft.runnerName != null &&
    !isOpenApiRunnerID(state.draft.runnerName)
  ) {
    return messages.unavailableRunnerValue;
  }

  const resolvedSelection = resolveEditableRunnerSelection(state);
  if (!getRunnerMetadata(resolvedSelection.runnerId)) {
    return messages.unavailableRunnerValue;
  }

  const runnerName =
    getRunnerDisplayName(resolvedSelection.runnerId) ??
    resolvedSelection.runnerId;
  const sourceLabel = messages.localizeRunnerSelectionSource(
    resolvedSelection.source,
  );

  return `${runnerName} (${sourceLabel})`;
}

function resolveEditableRunnerSelection(
  state: ReadyEditableConfigurationState,
) {
  return resolveRunnerSelection(
    state.draft.runnerName,
    state.initialValues.factoryRunnerName,
    state.initialValues.workerModelProvider,
  );
}
