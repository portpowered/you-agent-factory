import { Select } from "../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { cn } from "../../lib/cn";
import type { WorkstationDetailCardProps } from "./detail-card-types";
import type { getWorkstationDetailMessages } from "./messages";
import {
  getRunnerDisplayName,
  getRunnerMetadata,
  type RunnerID,
} from "./runner-metadata";

export function EditableConfigurationRunnerField({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const inheritedRunnerLabel = state.initialValues.factoryRunnerName
    ? messages.runnerInheritanceFactoryLabel(
        getRunnerDisplayName(state.initialValues.factoryRunnerName) ??
          state.initialValues.factoryRunnerName,
      )
    : messages.runnerInheritanceFactoryMissingLabel;

  const effectiveRunnerID =
    state.draft.runnerName ?? state.initialValues.factoryRunnerName ?? "codex";
  const runnerMetadata = getRunnerMetadata(effectiveRunnerID);
  const runnerName =
    getRunnerDisplayName(effectiveRunnerID) ?? effectiveRunnerID;
  const sourceLabel = state.draft.runnerName
    ? messages.runnerSelectionWorkstationLabel
    : state.initialValues.factoryRunnerName
      ? messages.runnerSelectionFactoryLabel
      : messages.runnerSelectionDefaultLabel;

  return (
    <div className="grid gap-2">
      <Select
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
      <p className={cn("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.runnerFieldHelp(runnerName, sourceLabel)}
      </p>
      {runnerMetadata ? (
        <div className="grid gap-2 rounded-xl border border-af-overlay/8 bg-af-overlay/4 p-3">
          <p className={cn("m-0 text-af-ink/72", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
            {messages.runnerCapabilitySupportHeading}
          </p>
          <ul className="m-0 grid list-none gap-2 p-0">
            {runnerMetadata.optionalCapabilities.map((capability) => (
              <li
                className="grid gap-1 rounded-lg border border-af-overlay/8 bg-af-surface/66 p-2"
                key={capability.capability}
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className={DASHBOARD_BODY_TEXT_CLASS}>
                    {labelForRunnerCapability(messages, capability.capability)}
                  </span>
                  <span
                    className={cn(
                      "rounded-full px-2 py-1 text-xs font-semibold",
                      capability.status === "supported"
                        ? "bg-af-success/10 text-af-success-ink"
                        : "bg-af-warning/12 text-af-warning-ink",
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
                      "m-0 text-af-ink/62",
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

  const effectiveRunnerID =
    state.draft.runnerName ??
    state.initialValues.factoryRunnerName ??
    state.initialValues.effectiveRunnerName;
  const runnerName = getRunnerDisplayName(effectiveRunnerID) ?? effectiveRunnerID;
  const sourceLabel = state.draft.runnerName
    ? messages.runnerInheritanceWorkstationSummaryLabel
    : state.initialValues.factoryRunnerName
      ? messages.runnerInheritanceFactorySummaryLabel
      : messages.runnerSelectionDefaultLabel;

  return `${runnerName} (${sourceLabel})`;
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
