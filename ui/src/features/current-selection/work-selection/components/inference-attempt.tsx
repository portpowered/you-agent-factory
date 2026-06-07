import { useId, useState } from "react";
import { surfacePanelVariants } from "../../../../components/ui";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
} from "../../base/components/current-selection-locale";
import type { InferenceAttemptCardProps } from "../lib/detail-card-types";
import { getInferenceAttemptTimingSummary } from "../lib/inference-attempt-timing";
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
  const [expanded, setExpanded] = useState(false);
  const attemptPanelId = useId();
  const summaryHeadingId = `${attemptPanelId}-heading`;
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
      className={surfacePanelVariants({
        className: "grid min-w-0 gap-2.5",
        radius: "lg",
        padding: "none"
      })}
    >
      <InferenceAttemptSummaryHeader
        attempt={attempt}
        expanded={expanded}
        headingId={summaryHeadingId}
        panelId={attemptPanelId}
        timingSummary={timingSummary}
        onToggle={() => setExpanded((current) => !current)}
      />
      {expanded ? (
        <section
          aria-labelledby={summaryHeadingId}
          className="grid gap-3"
          id={attemptPanelId}
        >
          <AttemptExpandedContent
            attempt={attempt}
            onSelectProviderSession={onSelectProviderSession}
            selectedProviderSessionKey={selectedProviderSessionKey}
          />
        </section>
      ) : null}
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
