import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_CARD_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  INFERENCE_REQUEST_PROMPT_LABEL,
  INFERENCE_RESPONSE_LABEL,
  InferenceAttemptDetail,
  InferenceAttemptTextSection,
} from "./detail-card-shared";
import type { InferenceAttemptCardProps } from "./detail-card-types";

export function InferenceAttemptCard({ attempt }: InferenceAttemptCardProps) {
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
        <InferenceAttemptDetail code label="dispatchId" value={attempt.dispatch_id} />
        <InferenceAttemptDetail code label="transitionId" value={attempt.transition_id} />
        <InferenceAttemptDetail code label="workingDirectory" value={attempt.working_directory} />
        <InferenceAttemptDetail code label="worktree" value={attempt.worktree} />
        <InferenceAttemptDetail code label="requestTime" value={attempt.request_time} />
        <InferenceAttemptDetail code label="outcome" value={attempt.outcome} />
        <InferenceAttemptDetail label="durationMillis" value={attempt.duration_millis} />
        <InferenceAttemptDetail code label="responseTime" value={attempt.response_time} />
        <InferenceAttemptDetail label="exitCode" value={attempt.exit_code} />
        <InferenceAttemptDetail code label="errorClass" value={attempt.error_class} />
      </dl>
      <InferenceAttemptTextSection label={INFERENCE_REQUEST_PROMPT_LABEL} value={attempt.prompt} />
      {attempt.response !== undefined ? (
        <InferenceAttemptTextSection label={INFERENCE_RESPONSE_LABEL} value={attempt.response} />
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

