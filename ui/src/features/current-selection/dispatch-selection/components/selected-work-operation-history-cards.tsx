import type { DashboardWorkMoveOperation } from "../../../../api/dashboard/types";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { formatLocalDateTime } from "../../../../components/ui/formatters";
import { cn } from "../../../../lib/cn";
import {
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
} from "../../base/components/current-selection-locale";
import {
  CURRENT_SELECTION_BADGE_CLASS,
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  PROVIDER_SESSION_CARD_CLASS,
} from "../../base/components/detail-card-shared";
import {
  requestOutcome,
  requestStartedAt,
  requestTitle,
} from "../dispatch-history/selected-work-dispatch-history-helpers";
import type { SelectedWorkRequestHistoryItem } from "../lib/detail-card-types";

const MOVE_OPERATION_KIND_BADGE_CLASS = cn(
  CURRENT_SELECTION_BADGE_CLASS,
  "border-af-info-border bg-af-info-surface text-af-info",
);

const WORKSTATION_OPERATION_KIND_BADGE_CLASS = cn(
  CURRENT_SELECTION_BADGE_CLASS,
  "border-af-border bg-af-surface-raised text-af-text-muted",
);

function OperationKindBadge({ label }: { label: string }) {
  return <span className={MOVE_OPERATION_KIND_BADGE_CLASS}>{label}</span>;
}

export function WorkstationOperationKindBadge({ label }: { label: string }) {
  return (
    <span className={WORKSTATION_OPERATION_KIND_BADGE_CLASS}>{label}</span>
  );
}

function formatMoveOccurredAt(
  move: DashboardWorkMoveOperation,
  unavailableValue: string,
  locale?: string | null,
): string {
  if (move.event_time) {
    return formatLocalDateTime(move.event_time, unavailableValue, locale);
  }

  return formatLocalDateTime(
    `${String(move.tick).padStart(12, "0")}:${String(move.sequence).padStart(12, "0")}`,
    unavailableValue,
    locale,
  );
}

export function OperatorMoveHistoryCard({
  move,
}: {
  move: DashboardWorkMoveOperation;
}) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const locale = useCurrentSelectionLocale();
  const transition = messages.formatMoveTransition(
    move.from_state,
    move.to_state,
  );

  return (
    <article
      aria-label={messages.operatorMoveRowAccessibleLabel(transition)}
      className={PROVIDER_SESSION_CARD_CLASS}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <strong className="min-w-0 [overflow-wrap:anywhere]">
            {messages.operatorMoveTitle}
          </strong>
          <OperationKindBadge label={messages.moveOperationKindBadge} />
        </div>
      </div>
      <dl className={cn("mt-2.5", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
        <InferenceAttemptDetail
          label={messages.moveTransitionLabel}
          value={transition}
        />
        <InferenceAttemptDetail
          label={messages.moveSourceLabel}
          value={messages.localizeMoveSource(move.source)}
        />
        <InferenceAttemptDetail
          label={messages.moveOccurredAtLabel}
          value={formatMoveOccurredAt(
            move,
            messages.workstationUnavailableValue,
            locale,
          )}
        />
      </dl>
    </article>
  );
}

export function LogicalMoveDispatchHistoryCard({
  currentDispatchID,
  request,
  selectedWorkID,
}: {
  currentDispatchID?: string | null;
  request: SelectedWorkRequestHistoryItem;
  selectedWorkID: string;
}) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();
  const locale = useCurrentSelectionLocale();
  const title = requestTitle(request, selectedWorkID);
  const outcome = requestOutcome(request);
  const isCurrentDispatch = currentDispatchID === request.dispatch_id;

  return (
    <article
      aria-label={messages.logicalMoveDispatchRowAccessibleLabel(
        request.workstation_name,
        request.dispatch_id,
      )}
      className={cn(
        PROVIDER_SESSION_CARD_CLASS,
        isCurrentDispatch &&
          "border-af-accent-border bg-af-accent-surface text-af-text",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <strong className="min-w-0 [overflow-wrap:anywhere]">
            {title || messages.logicalMoveDispatchTitle}
          </strong>
          <div className="flex flex-wrap items-center gap-2">
            <p
              className={cn(
                "m-0 text-af-text-muted",
                DASHBOARD_BODY_TEXT_CLASS,
              )}
            >
              {outcome
                ? enumMessages.localizeOutcome(outcome)
                : enumMessages.localizeOutcome("PENDING")}
            </p>
            <OperationKindBadge label={messages.moveOperationKindBadge} />
            {isCurrentDispatch ? (
              <span className={CURRENT_SELECTION_BADGE_CLASS}>
                {messages.currentDispatchBadge}
              </span>
            ) : null}
          </div>
        </div>
        <span className={EXECUTION_PILL_CLASS}>
          {request.dispatch_id || messages.unknownDispatchId}
        </span>
      </div>
      <dl className={cn("mt-2.5", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
        <InferenceAttemptDetail
          label={messages.workstationLabel}
          value={request.workstation_name}
        />
        <InferenceAttemptDetail
          label={messages.startedAtLabel}
          value={formatLocalDateTime(
            requestStartedAt(request),
            messages.workstationUnavailableValue,
            locale,
          )}
        />
      </dl>
    </article>
  );
}
