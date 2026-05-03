import { cx } from "../../lib/cx";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import type {
  SelectedWorkDispatchHistorySectionProps,
} from "./detail-card-types";
import { ProviderSessionAttempts } from "./provider-session-attempts";
import { DispatchHistoryCard } from "./selected-work-dispatch-history-card";

export function SelectedWorkDispatchHistorySection({
  activeTraceID,
  currentDispatchID,
  fallbackProviderSessions,
  onSelectTraceID,
  onSelectWorkID,
  requests,
  selectedWorkID,
  traceTargetId,
  workstationKind,
}: SelectedWorkDispatchHistorySectionProps) {
  if (requests.length === 0 && fallbackProviderSessions.length > 0) {
    return (
      <ProviderSessionAttempts
        attempts={fallbackProviderSessions}
        currentDispatchID={currentDispatchID}
        emptyMessage="No workstation dispatch has been recorded yet for this work item."
        onSelectWorkID={onSelectWorkID}
        renderHeading={(attempt) => attempt.workstation_name || attempt.transition_id}
        selectedWorkID={selectedWorkID}
        title="Workstation dispatches"
        workstationKind={workstationKind}
      />
    );
  }

  return (
    <section
      aria-labelledby="selected-work-dispatch-history-heading"
      className="mt-4 grid gap-[0.65rem]"
    >
      <div className="grid gap-[0.18rem]">
        <h4
          className={DASHBOARD_SECTION_HEADING_CLASS}
          id="selected-work-dispatch-history-heading"
        >
          Workstation dispatches
        </h4>
        <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {requests.length} {requests.length === 1 ? "dispatch" : "dispatches"}
        </p>
      </div>
      {requests.length > 0 ? (
        <div className="grid gap-[0.8rem]">
          {requests.map((request) => (
            <DispatchHistoryCard
              activeTraceID={activeTraceID}
              currentDispatchID={currentDispatchID}
              key={request.dispatch_id}
              onSelectTraceID={onSelectTraceID}
              onSelectWorkID={onSelectWorkID}
              request={request}
              selectedWorkID={selectedWorkID}
              traceTargetId={traceTargetId}
            />
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>
          No workstation dispatch has been recorded yet for this work item.
        </p>
      )}
    </section>
  );
}
