import type { DashboardInferenceAttempt } from "../../../../api/dashboard/types";
import {
  ExpandablePanelTrigger,
  surfacePanelVariants,
} from "../../../../components/ui";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionOperationalEnumMessages,
} from "../../base/components/current-selection-locale";
import { CurrentSelectionExecutionPill } from "../../base/components/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../base/public";

export interface InferenceAttemptSummaryHeaderProps {
  attempt: DashboardInferenceAttempt;
  expanded: boolean;
  headingId: string;
  onToggle: () => void;
  panelId: string;
  timingSummary?: string;
}

export function InferenceAttemptSummaryHeader({
  attempt,
  expanded,
  headingId,
  onToggle,
  panelId,
  timingSummary,
}: InferenceAttemptSummaryHeaderProps) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();

  return (
    <div
      className={surfacePanelVariants({
        className:
          "flex items-center justify-between gap-3 px-3 py-2 [&_h4]:m-0",
        radius: "lg",
      })}
    >
      <div className="grid min-w-0 gap-1">
        <div className="flex items-start justify-between gap-3">
          <strong id={headingId}>
            {detailMessages.attemptTitle(attempt.attempt)}
          </strong>
          <CurrentSelectionExecutionPill>
            {attempt.outcome
              ? enumMessages.localizeOutcome(attempt.outcome)
              : enumMessages.localizeOutcome("PENDING")}
          </CurrentSelectionExecutionPill>
        </div>
        {timingSummary ? (
          <CurrentSelectionSupportingText tone="status">
            {timingSummary}
          </CurrentSelectionSupportingText>
        ) : null}
      </div>
      <ExpandablePanelTrigger
        aria-label={
          expanded
            ? detailMessages.collapseAttemptAction(attempt.attempt)
            : detailMessages.expandAttemptAction(attempt.attempt)
        }
        controlsID={panelId}
        expanded={expanded}
        onClick={onToggle}
        type="button"
        variant="section"
      >
        {expanded
          ? detailMessages.collapseAttemptAction(attempt.attempt)
          : detailMessages.expandAttemptAction(attempt.attempt)}
      </ExpandablePanelTrigger>
    </div>
  );
}
