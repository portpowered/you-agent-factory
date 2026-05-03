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
import type { InferenceAttemptCardProps } from "./detail-card-types";

export function InferenceAttemptCard({ attempt }: InferenceAttemptCardProps) {
  const provider = attempt.diagnostics?.provider?.provider ?? attempt.provider_session?.provider;
  const model = attempt.diagnostics?.provider?.model;
  const providerSessionLogTarget = getProviderSessionLogTarget(
    attempt.provider_session,
    attempt.request_time,
  );

  return (
    <article
      aria-label={`Inference attempt ${attempt.attempt}`}
      className={INFERENCE_ATTEMPT_CARD_CLASS}
    >
      <div className="flex items-start justify-between gap-[0.8rem]">
        <strong>Attempt {attempt.attempt}</strong>
        <span className={EXECUTION_PILL_CLASS}>{attempt.outcome ?? "PENDING"}</span>
      </div>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <InferenceAttemptDetail
          code
          label="inferenceRequestId"
          value={attempt.inference_request_id}
        />
        <InferenceAttemptDetail
          code={!providerSessionLogTarget}
          label="providerSession"
          value={
            attempt.provider_session ? formatProviderSession(attempt.provider_session) : undefined
          }
        />
        <InferenceAttemptDetail code label="provider" value={provider} />
        <InferenceAttemptDetail code label="model" value={model} />
        <InferenceAttemptDetail code label="dispatchId" value={attempt.dispatch_id} />
        <InferenceAttemptDetail code label="transitionId" value={attempt.transition_id} />
        <InferenceAttemptDetail code label="workingDirectory" value={attempt.working_directory} />
        <InferenceAttemptDetail code label="worktree" value={attempt.worktree} />
        <InferenceAttemptDetail code label="requestTime" value={attempt.request_time} />
        <InferenceAttemptDetail code label="outcome" value={attempt.outcome} />
        <InferenceAttemptDetail
          label="elapsedTime"
          value={
            attempt.duration_millis !== undefined
              ? formatDurationMillis(attempt.duration_millis)
              : undefined
          }
        />
        <InferenceAttemptDetail code label="responseTime" value={attempt.response_time} />
        <InferenceAttemptDetail label="exitCode" value={attempt.exit_code} />
        <InferenceAttemptDetail code label="errorClass" value={attempt.error_class} />
      </dl>
      <InferenceAttemptTextSection label="Request body" value={attempt.prompt} />
      {attempt.response !== undefined ? (
        <InferenceAttemptTextSection label="Response body" value={attempt.response} />
      ) : attempt.outcome ? (
        <p className={DETAIL_COPY_CLASS}>
          Provider response text is not available for this inference attempt.
        </p>
      ) : (
        <p className={DETAIL_COPY_CLASS}>Awaiting provider response.</p>
      )}
    </article>
  );
}
