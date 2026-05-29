import { Select } from "../../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import { resolveRunnerSelection } from "../../../current-factory-definition/lib/runner-selection";
import type { WorkstationDetailCardProps } from "../lib/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";
import {
  getRunnerDisplayName,
  getRunnerMetadata,
  type RunnerID,
} from "../editing/runner-metadata";
import { isOpenApiRunnerID } from "../messages/runner-openapi-enums";

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

  const runnerMetadata = getRunnerMetadata(resolvedSelection.runnerId);
  const runnerName =
    getRunnerDisplayName(resolvedSelection.runnerId) ?? resolvedSelection.runnerId;
  const sourceLabel = messages.localizeRunnerSelectionSource(
    resolvedSelection.source,
  );

  return (
    <div className="grid gap-2">
      <Select
        aria-describedby={
          state.validationErrors.runnerName
            ? "editable-workstation-runner-error"
            : undefined
        }
        aria-invalid={state.validationErrors.runnerName ? "true" : undefined}
        className={DASHBOARD_BODY_TEXT_CLASS}
        id="editable-workstation-runner"
        onChange={(event) =>
          state.onRunnerChange(
            event.target.value === ""
              ? null
              : (event.target.value as RunnerID),
          )
        }
        value={state.draft.runnerName ?? ""}
      >
        <option value="">{inheritedRunnerLabel}</option>
        {state.initialValues.runnerOptions.map((runnerID) => (
          <option key={runnerID} value={runnerID}>
            {getRunnerDisplayName(runnerID) ?? runnerID}
          </option>
        ))}
      </Select>
      <p className={cn("m-0 text-af-text-subtle", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.runnerFieldHelp(runnerName, sourceLabel)}
      </p>
      {runnerMetadata ? (
        <div className="grid gap-2 rounded-xl border border-af-border bg-af-surface-subtle p-3">
          <p className={cn("m-0 text-af-text-muted", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
            {messages.runnerCapabilitySupportHeading}
          </p>
          <ul className="m-0 grid list-none gap-2 p-0">
            {runnerMetadata.capabilities.optionalCapabilities.map((capability) => (
              <li
                className="grid gap-1 rounded-lg border border-af-border bg-af-surface-raised p-2"
                key={capability.capability}
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className={cn("text-af-text", DASHBOARD_BODY_TEXT_CLASS)}>
                    {labelForRunnerCapability(messages, capability.capability)}
                  </span>
                  <span
                    className={cn(
                      "rounded-full border px-2 py-1 text-xs font-semibold",
                      capability.status === "supported"
                        ? "border-af-success-border bg-af-success-surface text-af-success"
                        : "border-af-warning-border bg-af-warning-surface text-af-warning",
                    )}
                  >
                    {capability.status === "supported"
                      ? messages.runnerCapabilitySupportedLabel
                      : messages.runnerCapabilityUnsupportedLabel}
                  </span>
                </div>
                {capability.detail ? (
                  <p
                    className={cn(
                      "m-0 text-af-text-subtle",
                      DASHBOARD_SUPPORTING_TEXT_CLASS,
                    )}
                  >
                    {capability.detail}
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

export function resolveWorkstationSummaryRunnerValue(
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
    getRunnerDisplayName(resolvedSelection.runnerId) ?? resolvedSelection.runnerId;
  const sourceLabel = messages.localizeRunnerSelectionSource(
    resolvedSelection.source,
  );

  return `${runnerName} (${sourceLabel})`;
}

function resolveEditableRunnerSelection(state: ReadyEditableConfigurationState) {
  return resolveRunnerSelection(
    state.draft.runnerName,
    state.initialValues.factoryRunnerName,
    state.initialValues.workerModelProvider,
  );
}

function labelForRunnerCapability(
  messages: ReturnType<typeof getWorkstationDetailMessages>,
  capability:
    | "image_input"
    | "session_resume"
    | "structured_output"
    | "working_directory"
    | "worktree",
) {
  switch (capability) {
    case "image_input":
      return messages.runnerCapabilityImageInputLabel;
    case "session_resume":
      return messages.runnerCapabilitySessionResumeLabel;
    case "structured_output":
      return messages.runnerCapabilityStructuredOutputLabel;
    case "working_directory":
      return messages.runnerCapabilityWorkingDirectoryLabel;
    case "worktree":
      return messages.runnerCapabilityWorktreeLabel;
  }
}
