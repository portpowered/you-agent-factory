import type { DashboardInferenceAttempt } from "../../../../../api/dashboard/types";
import { surfacePanelVariants } from "@you-agent-factory/components/layout";
import type { LoadableProviderSessionRef } from "../../../../provider-session-detail/lib/provider-session-ref";
import { useCurrentSelectionOperationalEnumMessages } from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionExecutionPill } from "../../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../../base/components/presentation/current-selection-supporting-text";
import { InferenceAttemptProviderSessionPreview } from "./inference-attempt-provider-session";

export interface InferenceAttemptSummaryHeaderProps {
  attempt: DashboardInferenceAttempt;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  selectedProviderSessionKey?: string | null;
  timingSummary?: string;
}

export function InferenceAttemptSummaryHeader({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
  timingSummary,
}: InferenceAttemptSummaryHeaderProps) {
  const enumMessages = useCurrentSelectionOperationalEnumMessages();

  return (
    <div
      className={surfacePanelVariants({
        className: "grid gap-3 px-3 py-2",
        radius: "lg",
      })}
    >
      <div className="flex flex-wrap items-center gap-3">
        <CurrentSelectionExecutionPill>
          {attempt.outcome
            ? enumMessages.localizeOutcome(attempt.outcome)
            : enumMessages.localizeOutcome("PENDING")}
        </CurrentSelectionExecutionPill>
        {timingSummary ? (
          <CurrentSelectionSupportingText tone="status">
            {timingSummary}
          </CurrentSelectionSupportingText>
        ) : null}
      </div>
      <InferenceAttemptProviderSessionPreview
        attempt={attempt}
        onSelectProviderSession={onSelectProviderSession}
        selectedProviderSessionKey={selectedProviderSessionKey}
      />
    </div>
  );
}
