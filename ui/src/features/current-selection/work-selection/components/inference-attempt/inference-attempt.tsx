import { useId } from "react";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
} from "../../../base/components/presentation/current-selection-locale";
import type { InferenceAttemptCardProps } from "../../lib/detail-card-types";
import { getInferenceAttemptTimingSummary } from "../../lib/inference-attempt-timing";
import {
  InferenceAttemptRequestBodySection,
  InferenceAttemptResponseSection,
} from "./inference-attempt-body-sections";
import { InferenceAttemptMetadataDetails } from "./inference-attempt-metadata-details";
import { InferenceAttemptProviderSessionDetails } from "./inference-attempt-provider-session";
import { InferenceAttemptSummaryHeader } from "./inference-attempt-summary-header";

export function InferenceAttemptCard({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: InferenceAttemptCardProps) {
  const attemptPanelId = useId();
  const detailMessages = useCurrentSelectionDetailMessages();
  const locale = useCurrentSelectionLocale();
  const timingSummary = getInferenceAttemptTimingSummary(
    attempt,
    detailMessages,
    locale,
  );

  return (
    <article
      aria-label={detailMessages.attemptAriaLabel(attempt.attempt)}
      className="grid min-w-0 gap-2.5"
    >
      <CurrentSelectionExpandableSection
        className="mt-0"
        contentId={attemptPanelId}
        headingId={`${attemptPanelId}-heading`}
        preview={
          <InferenceAttemptSummaryHeader
            attempt={attempt}
            onSelectProviderSession={onSelectProviderSession}
            selectedProviderSessionKey={selectedProviderSessionKey}
            timingSummary={timingSummary}
          />
        }
        title={detailMessages.attemptTitle(attempt.attempt)}
        toggleLabel={(expanded) =>
          expanded
            ? detailMessages.collapseAttemptAction(attempt.attempt)
            : detailMessages.expandAttemptAction(attempt.attempt)
        }
      >
        <AttemptExpandedContent
          attempt={attempt}
          onSelectProviderSession={onSelectProviderSession}
          selectedProviderSessionKey={selectedProviderSessionKey}
        />
      </CurrentSelectionExpandableSection>
    </article>
  );
}

function AttemptExpandedContent({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: InferenceAttemptCardProps) {
  return (
    <>
      <InferenceAttemptMetadataDetails attempt={attempt} />
      <InferenceAttemptProviderSessionDetails
        attempt={attempt}
        onSelectProviderSession={onSelectProviderSession}
        selectedProviderSessionKey={selectedProviderSessionKey}
      />
      <InferenceAttemptRequestBodySection attempt={attempt} />
      <InferenceAttemptResponseSection attempt={attempt} />
    </>
  );
}
