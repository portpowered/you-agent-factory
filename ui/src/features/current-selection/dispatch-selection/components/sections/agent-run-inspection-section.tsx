import type { DashboardAgentRunInspection } from "../../../../../api/dashboard/agent-run-inspection-types";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import { CurrentSelectionDetailItem } from "../../../base/components/detail/current-selection-detail-item";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionLabel } from "../../../base/components/presentation/current-selection-label";
import { useCurrentSelectionDetailMessages } from "../../../base/components/presentation/current-selection-locale";

export function AgentRunInspectionSection({
  inspection,
}: {
  inspection: DashboardAgentRunInspection | undefined;
}) {
  const messages = useCurrentSelectionDetailMessages();

  return (
    <CurrentSelectionExpandableSection
      defaultExpanded
      title={messages.agentRunInspectionHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {!inspection ? (
        <p className="text-sm text-muted-foreground">
          {messages.agentRunInspectionEmpty}
        </p>
      ) : (
        <div className="grid gap-4">
          <CurrentSelectionDescriptionList>
            {inspection.tool_policy ? (
              <CurrentSelectionDetailItem
                label={messages.agentToolPolicyLabel}
                value={inspection.tool_policy}
              />
            ) : null}
            {inspection.failure_class ? (
              <CurrentSelectionDetailItem
                label={messages.agentFailureClassLabel}
                value={inspection.failure_class}
              />
            ) : null}
            {inspection.recovery_action ? (
              <CurrentSelectionDetailItem
                label={messages.agentRecoveryActionLabel}
                value={inspection.recovery_action}
              />
            ) : null}
          </CurrentSelectionDescriptionList>
          <AgentRunToolDiagnosticsList
            diagnostics={inspection.tool_diagnostics}
            emptyMessage={messages.agentToolDiagnosticsEmpty}
            heading={messages.agentToolDiagnosticsHeading}
          />
          <AgentRunTranscriptList
            entries={inspection.transcript}
            emptyMessage={messages.agentTranscriptEmpty}
            heading={messages.agentTranscriptHeading}
          />
        </div>
      )}
    </CurrentSelectionExpandableSection>
  );
}

function AgentRunToolDiagnosticsList({
  diagnostics,
  emptyMessage,
  heading,
}: {
  diagnostics: DashboardAgentRunInspection["tool_diagnostics"];
  emptyMessage: string;
  heading: string;
}) {
  if (!diagnostics || diagnostics.length === 0) {
    return (
      <div className="grid gap-1">
        <CurrentSelectionLabel>{heading}</CurrentSelectionLabel>
        <p className="text-sm text-muted-foreground">{emptyMessage}</p>
      </div>
    );
  }

  return (
    <div className="grid gap-2">
      <CurrentSelectionLabel>{heading}</CurrentSelectionLabel>
      <ul className="grid gap-2">
        {diagnostics.map((entry) => (
          <li
            className="rounded-md border border-border px-3 py-2 text-sm"
            key={`${entry.tool_name ?? "tool"}::${entry.phase ?? "phase"}::${entry.detail ?? ""}`}
          >
            <div className="font-medium">
              {[entry.tool_name, entry.phase].filter(Boolean).join(" · ")}
            </div>
            {entry.detail ? (
              <div className="mt-1 text-muted-foreground [overflow-wrap:anywhere]">
                {entry.detail}
              </div>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

function AgentRunTranscriptList({
  entries,
  emptyMessage,
  heading,
}: {
  entries: DashboardAgentRunInspection["transcript"];
  emptyMessage: string;
  heading: string;
}) {
  if (!entries || entries.length === 0) {
    return (
      <div className="grid gap-1">
        <CurrentSelectionLabel>{heading}</CurrentSelectionLabel>
        <p className="text-sm text-muted-foreground">{emptyMessage}</p>
      </div>
    );
  }

  return (
    <div className="grid gap-2">
      <CurrentSelectionLabel>{heading}</CurrentSelectionLabel>
      <ul className="grid gap-2">
        {entries.map((entry) => (
          <li
            className="rounded-md border border-border px-3 py-2 text-sm"
            key={`${entry.role ?? "role"}::${entry.summary ?? ""}`}
          >
            <div className="font-medium">{entry.role ?? "message"}</div>
            {entry.summary ? (
              <div className="mt-1 text-muted-foreground [overflow-wrap:anywhere]">
                {entry.summary}
              </div>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
