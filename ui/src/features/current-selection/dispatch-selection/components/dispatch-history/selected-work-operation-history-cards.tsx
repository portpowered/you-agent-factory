import type { DashboardWorkMoveOperation } from "../../../../../api/dashboard/types";
import { formatLocalDateTime } from "../../../../../components/ui/formatters";
import {
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
} from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionBadge } from "../../../base/components/presentation/current-selection-pill";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "../../../history/components/current-selection-history-card";
import { InferenceAttemptDetail } from "../../../work-selection/components/inference-attempt/inference-attempt-detail";
import {
  requestOutcome,
  requestStartedAt,
  requestTitle,
} from "../../dispatch-history/selected-work-dispatch-history-helpers";
import type { SelectedWorkRequestHistoryItem } from "../../lib/detail-card-types";

function OperationKindBadge({ label }: { label: string }) {
  return <CurrentSelectionBadge tone="info">{label}</CurrentSelectionBadge>;
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
      <CurrentSelectionHistoryCardHeader
        badges={<OperationKindBadge label={messages.moveOperationKindBadge} />}
        title={messages.operatorMoveTitle}
      />
      <CurrentSelectionDescriptionList className="mt-2.5">
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
      </CurrentSelectionDescriptionList>
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
      <CurrentSelectionHistoryCardHeader
        badges={
          <>
            <OperationKindBadge label={messages.moveOperationKindBadge} />
            {isCurrentDispatch ? (
              <CurrentSelectionBadge>
                {messages.currentDispatchBadge}
              </CurrentSelectionBadge>
            ) : null}
          </>
        }
        identifier={request.dispatch_id || messages.unknownDispatchId}
        subtitle={
          outcome
            ? enumMessages.localizeOutcome(outcome)
            : enumMessages.localizeOutcome("PENDING")
        }
        title={title || messages.logicalMoveDispatchTitle}
      />
      <CurrentSelectionDescriptionList className="mt-2.5">
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
      </CurrentSelectionDescriptionList>
    </CurrentSelectionHistoryCard>
  );
}
