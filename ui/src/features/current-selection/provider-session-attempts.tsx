import { useEffect, useState } from "react";

import {
  formatProviderSession,
  formatWorkstationRunOutcome,
  getProviderSessionLogTarget,
} from "../../components/dashboard/formatters";
import { cx } from "../../components/dashboard/classnames";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  EXECUTION_PILL_CLASS,
  HISTORY_HEADER_CLASS,
  HISTORY_TOGGLE_CLASS,
  PROVIDER_SESSION_CARD_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";
import type {
  CollapsibleProviderSessionAttemptsProps,
  ProviderSessionAttemptsProps,
  ProviderSessionLogAccessProps,
} from "./detail-card-types";

export function CollapsibleProviderSessionAttempts({
  attempts,
  emptyMessage,
  onSelectWorkID,
  onSelectWorkstationRequest,
  renderHeading,
  resetKey,
  selectedRequestDispatchID,
  selectedWorkID,
  title = "Run history",
  workstationKind,
  workstationRequestsByDispatchID,
}: CollapsibleProviderSessionAttemptsProps) {
  const [expanded, setExpanded] = useState(false);
  const historyID = `workstation-run-history-${resetKey}`;
  const itemCountLabel = `${attempts.length} ${attempts.length === 1 ? "run" : "runs"}`;

  useEffect(() => {
    setExpanded(false);
  }, [resetKey]);

  return (
    <section aria-labelledby={`${historyID}-heading`} className="mt-4 grid gap-[0.65rem]">
      <div className={HISTORY_HEADER_CLASS}>
        <div className="grid min-w-0 gap-[0.18rem]">
          <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={`${historyID}-heading`}>
            {title}
          </h4>
          <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
            {itemCountLabel}
          </p>
        </div>
        <button
          aria-controls={historyID}
          aria-expanded={expanded}
          className={HISTORY_TOGGLE_CLASS}
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? "Collapse" : "Expand"}
        </button>
      </div>
      {expanded ? (
        <div id={historyID}>
          <ProviderSessionAttemptList
            attempts={attempts}
            emptyMessage={emptyMessage}
            onSelectWorkID={onSelectWorkID}
            onSelectWorkstationRequest={onSelectWorkstationRequest}
            renderHeading={renderHeading}
            selectedRequestDispatchID={selectedRequestDispatchID}
            selectedWorkID={selectedWorkID}
            workstationKind={workstationKind}
            workstationRequestsByDispatchID={workstationRequestsByDispatchID}
          />
        </div>
      ) : null}
    </section>
  );
}

export function ProviderSessionAttempts({
  attempts,
  emptyMessage,
  onSelectWorkID,
  onSelectWorkstationRequest,
  renderHeading,
  selectedRequestDispatchID,
  selectedWorkID,
  title = "Workstation dispatches",
  workstationKind,
  workstationRequestsByDispatchID,
}: ProviderSessionAttemptsProps) {
  return (
    <section className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{title}</h4>
      <ProviderSessionAttemptList
        attempts={attempts}
        emptyMessage={emptyMessage}
        onSelectWorkID={onSelectWorkID}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        renderHeading={renderHeading}
        selectedRequestDispatchID={selectedRequestDispatchID}
        selectedWorkID={selectedWorkID}
        workstationKind={workstationKind}
        workstationRequestsByDispatchID={workstationRequestsByDispatchID}
      />
    </section>
  );
}

function ProviderSessionAttemptList({
  attempts,
  emptyMessage,
  onSelectWorkID,
  onSelectWorkstationRequest,
  renderHeading,
  selectedRequestDispatchID,
  selectedWorkID,
  workstationKind,
  workstationRequestsByDispatchID,
}: ProviderSessionAttemptsProps) {
  if (attempts.length === 0) {
    return <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>;
  }

  return (
    <div className="grid gap-[0.8rem]">
      {attempts.map((attempt) => {
        const outcome = formatWorkstationRunOutcome(attempt.outcome, { workstationKind });
        const request = workstationRequestsByDispatchID?.[attempt.dispatch_id];
        const requestSelected = selectedRequestDispatchID === attempt.dispatch_id;

        return (
          <article
            className={PROVIDER_SESSION_CARD_CLASS}
            key={`${attempt.dispatch_id}-${attempt.provider_session?.id}`}
          >
            <div className="flex items-start justify-between gap-[0.8rem]">
              <strong>{renderHeading(attempt)}</strong>
              <span className={EXECUTION_PILL_CLASS}>{attempt.dispatch_id}</span>
            </div>
            <div className="mt-[0.45rem] grid gap-[0.18rem]">
              <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
                {outcome.label}
              </p>
              {outcome.rawOutcomeLabel ? (
                <p className={cx("m-0 text-af-code-ink/72", DASHBOARD_SUPPORTING_CODE_CLASS)}>
                  {outcome.rawOutcomeLabel}
                </p>
              ) : null}
            </div>
            <ProviderSessionLogAccess
              session={attempt.provider_session}
              startedAt={attempt.diagnostics?.provider?.request_metadata?.request_time}
            />
            <div className="mt-[0.55rem] grid gap-[0.45rem]">
              {attempt.work_items && attempt.work_items.length > 0 ? (
                onSelectWorkID ? (
                  attempt.work_items.map((workItem) => {
                    const selected = selectedWorkID === workItem.work_id;

                    return (
                      <button
                        aria-label={`Select work item ${workItem.display_name || workItem.work_id}`}
                        aria-pressed={selected}
                        className={WORK_SELECTION_BUTTON_CLASS}
                        key={`${attempt.dispatch_id}-${workItem.work_id}`}
                        onClick={() => onSelectWorkID(workItem.work_id)}
                        type="button"
                      >
                        {selected ? "Work selected" : `Open ${workItem.display_name || workItem.work_id}`}
                      </button>
                    );
                  })
                ) : null
              ) : (
                <p className={REQUEST_SELECTION_STATUS_CLASS}>
                  Work details unavailable for dispatch{" "}
                  <code className={RUNTIME_DETAIL_CODE_CLASS}>{attempt.dispatch_id}</code>.
                </p>
              )}
              {onSelectWorkstationRequest ? (
                request ? (
                  <button
                    aria-label={`Select workstation request ${request.dispatch_id}`}
                    aria-pressed={requestSelected}
                    className={WORK_SELECTION_BUTTON_CLASS}
                    onClick={() => onSelectWorkstationRequest(request)}
                    type="button"
                  >
                    {requestSelected ? "Request selected" : "Open request details"}
                  </button>
                ) : (
                  <p className={REQUEST_SELECTION_STATUS_CLASS}>
                    Request details unavailable for dispatch{" "}
                    <code className={RUNTIME_DETAIL_CODE_CLASS}>{attempt.dispatch_id}</code>.
                  </p>
                )
              ) : null}
            </div>
          </article>
        );
      })}
    </div>
  );
}

function ProviderSessionLogAccess({ session, startedAt }: ProviderSessionLogAccessProps) {
  const logTarget = getProviderSessionLogTarget(session, startedAt);
  const metadata = formatProviderSession(session);

  return (
    <div className="mt-[0.45rem] grid min-w-0 gap-[0.3rem]">
      {logTarget ? (
        <a
          className={cx(
            "w-fit rounded-lg font-bold text-af-accent underline underline-offset-4 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          href={logTarget.href}
          title={logTarget.display}
        >
          Codex session log
        </a>
      ) : (
        <span className={cx("font-bold text-af-ink/78", DASHBOARD_BODY_TEXT_CLASS)}>
          Session log unavailable
        </span>
      )}
      <code
        className={cx(
          "inline-block text-af-code-ink/78 [overflow-wrap:anywhere]",
          DASHBOARD_BODY_CODE_CLASS,
        )}
      >
        {metadata}
      </code>
    </div>
  );
}
