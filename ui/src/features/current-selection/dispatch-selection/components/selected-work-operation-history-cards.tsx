import type { DashboardWorkMoveOperation } from "../../../../api/dashboard/types";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { formatLocalDateTime } from "../../../../components/ui/formatters";
import { cn } from "../../../../lib/cn";
import { CurrentSelectionHistoryCard } from "../../base/components/current-selection-history-card";
import {
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
} from "../../base/components/current-selection-locale";
import {
  CurrentSelectionBadge,
  CurrentSelectionExecutionPill,
} from "../../base/components/current-selection-pill";
import { INFERENCE_ATTEMPT_DETAIL_CLASS } from "../../base/components/detail-card-shared";
import { InferenceAttemptDetail } from "../../base/components/inference-attempt-detail";
import {
  requestOutcome,
  requestStartedAt,
  requestTitle,
} from "../dispatch-history/selected-work-dispatch-history-helpers";
import type { SelectedWorkRequestHistoryItem } from "../lib/detail-card-types";

function OperationKindBadge({ label }: { label: string }) {
  return <CurrentSelectionBadge tone="info">{label}</CurrentSelectionBadge>;
}

export function WorkstationOperationKindBadge({ label }: { label: string }) {
  return <CurrentSelectionBadge tone="neutral">{label}</CurrentSelectionBadge>;
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
    <CurrentSelectionHistoryCard
      aria-label={messages.operatorMoveRowAccessibleLabel(transition)}
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
    </CurrentSelectionHistoryCard>
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
    <CurrentSelectionHistoryCard
      aria-label={messages.logicalMoveDispatchRowAccessibleLabel(
        request.workstation_name,
        request.dispatch_id,
      )}
      className={isCurrentDispatch ? "text-on-surface" : undefined}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <strong className="min-w-0 [overflow-wrap:anywhere]">
            {title || messages.logicalMoveDispatchTitle}
          </strong>
          <div className="flex flex-wrap items-center gap-2">
            <p
              className={cn(
                "m-0 text-on-surface-variant",
                DASHBOARD_BODY_TEXT_CLASS,
              )}
            >
              {outcome
                ? enumMessages.localizeOutcome(outcome)
                : enumMessages.localizeOutcome("PENDING")}
            </p>
            <OperationKindBadge label={messages.moveOperationKindBadge} />
            {isCurrentDispatch ? (
              <CurrentSelectionBadge>
                {messages.currentDispatchBadge}
              </CurrentSelectionBadge>
            ) : null}
          </div>
        </div>
        <CurrentSelectionExecutionPill>
          {request.dispatch_id || messages.unknownDispatchId}
        </CurrentSelectionExecutionPill>
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
    </CurrentSelectionHistoryCard>
  );
}
