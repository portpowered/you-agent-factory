import {
  formatDurationMillis,
  formatProviderSession,
  getProviderSessionLogTarget,
} from "../../components/ui/formatters";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_CARD_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  InferenceAttemptTextSection,
} from "./detail-card-shared";
import { useCurrentSelectionDetailMessages } from "./current-selection-locale";
import type { InferenceAttemptCardProps } from "./detail-card-types";

export function InferenceAttemptCard({ attempt }: InferenceAttemptCardProps) {
  const messages = useCurrentSelectionDetailMessages();
  const provider = attempt.diagnostics?.provider?.provider ?? attempt.provider_session?.provider;
  const model = attempt.diagnostics?.provider?.model;
  const providerSessionLogTarget = getProviderSessionLogTarget(
    attempt.provider_session,
    attempt.request_time,
  );

  return (
    <article
      aria-label={messages.attemptAriaLabel(attempt.attempt)}
      className={INFERENCE_ATTEMPT_CARD_CLASS}
    >
      <div className="flex items-start justify-between gap-3">
        <strong>{messages.attemptTitle(attempt.attempt)}</strong>
        <span className={EXECUTION_PILL_CLASS}>
          {attempt.outcome ?? messages.pendingOutcome}
        </span>
      </div>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <InferenceAttemptDetail
          code
          label={messages.inferenceRequestIdLabel}
          value={attempt.inference_request_id}
        />
        <InferenceAttemptDetail
          code={!providerSessionLogTarget}
          label={messages.providerSessionLabel}
          value={
            attempt.provider_session ? formatProviderSession(attempt.provider_session) : undefined
          }
        />
        <InferenceAttemptDetail code label={messages.providerLabel} value={provider} />
        <InferenceAttemptDetail code label={messages.modelLabel} value={model} />
        <InferenceAttemptDetail code label={messages.dispatchIdLabel} value={attempt.dispatch_id} />
        <InferenceAttemptDetail code label={messages.transitionIdLabel} value={attempt.transition_id} />
        <InferenceAttemptDetail
          code
          label={messages.workingDirectoryLabel}
          value={attempt.working_directory}
        />
        <InferenceAttemptDetail code label={messages.worktreeLabel} value={attempt.worktree} />
        <InferenceAttemptDetail code label={messages.requestTimeLabel} value={attempt.request_time} />
        <InferenceAttemptDetail code label={messages.outcomeLabel} value={attempt.outcome} />
        <InferenceAttemptDetail
          label={messages.elapsedTimeLabel}
          value={
            attempt.duration_millis !== undefined
              ? formatDurationMillis(attempt.duration_millis)
              : undefined
          }
        />
        <InferenceAttemptDetail code label={messages.responseTimeLabel} value={attempt.response_time} />
        <InferenceAttemptDetail label={messages.exitCodeLabel} value={attempt.exit_code} />
        <InferenceAttemptDetail code label={messages.errorClassLabel} value={attempt.error_class} />
      </dl>
      <InferenceAttemptTextSection label={messages.requestBodyLabel} value={attempt.prompt} />
      {attempt.response !== undefined ? (
        <InferenceAttemptTextSection label={messages.responseBodyLabel} value={attempt.response} />
      ) : attempt.outcome ? (
        <p className={DETAIL_COPY_CLASS}>{messages.providerResponseUnavailable}</p>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.awaitingProviderResponse}</p>
      )}
    </article>
  );
}
