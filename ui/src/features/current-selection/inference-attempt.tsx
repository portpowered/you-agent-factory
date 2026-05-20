import {
  formatDurationMillis,
  formatProviderSession,
  getProviderSessionLogTarget,
} from "../../components/ui/formatters";
import { cn } from "../../lib/cn";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_CARD_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  InferenceAttemptTextSection,
  PROVIDER_SESSION_SELECTION_BUTTON_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
} from "./detail-card-shared";
import type { InferenceAttemptCardProps } from "./detail-card-types";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionWorkstationDetailMessages,
} from "./current-selection-locale";
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "./provider-session-details";

export function InferenceAttemptCard({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: InferenceAttemptCardProps) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const workstationMessages = useCurrentSelectionWorkstationDetailMessages();
  const provider = attempt.diagnostics?.provider?.provider ?? attempt.provider_session?.provider;
  const model = attempt.diagnostics?.provider?.model;
  const providerSessionLogTarget = getProviderSessionLogTarget(
    attempt.provider_session,
    attempt.request_time,
  );
  const loadableProviderSession = getLoadableProviderSessionRef({
    dispatch_id: attempt.dispatch_id,
    provider_session: attempt.provider_session,
  });
  const providerSessionLabel = attempt.provider_session
    ? formatProviderSession(attempt.provider_session)
    : undefined;
  const providerSessionSelected =
    loadableProviderSession !== null &&
    selectedProviderSessionKey ===
      providerSessionSelectionKey(loadableProviderSession);

  return (
    <article
      aria-label={detailMessages.attemptAriaLabel(attempt.attempt)}
      className={INFERENCE_ATTEMPT_CARD_CLASS}
    >
      <div className="flex items-start justify-between gap-3">
        <strong>{detailMessages.attemptTitle(attempt.attempt)}</strong>
        <span className={EXECUTION_PILL_CLASS}>
          {attempt.outcome ?? detailMessages.pendingOutcome}
        </span>
      </div>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <InferenceAttemptDetail
          code
          label={detailMessages.inferenceRequestIdLabel}
          value={attempt.inference_request_id}
        />
        <InferenceAttemptDetail code label={detailMessages.providerLabel} value={provider} />
        <InferenceAttemptDetail code label={detailMessages.modelLabel} value={model} />
        <InferenceAttemptDetail
          code
          label={detailMessages.workingDirectoryLabel}
          value={attempt.working_directory}
        />
        <InferenceAttemptDetail code label={detailMessages.worktreeLabel} value={attempt.worktree} />
        <InferenceAttemptDetail code label={detailMessages.requestTimeLabel} value={attempt.request_time} />
        <InferenceAttemptDetail code label={detailMessages.outcomeLabel} value={attempt.outcome} />
        <InferenceAttemptDetail
          label={detailMessages.elapsedTimeLabel}
          value={
            attempt.duration_millis !== undefined
              ? formatDurationMillis(attempt.duration_millis)
              : undefined
          }
        />
        <InferenceAttemptDetail code label={detailMessages.responseTimeLabel} value={attempt.response_time} />
        <InferenceAttemptDetail label={detailMessages.exitCodeLabel} value={attempt.exit_code} />
        <InferenceAttemptDetail code label={detailMessages.errorClassLabel} value={attempt.error_class} />
      </dl>
      {providerSessionLabel ? (
        loadableProviderSession && onSelectProviderSession ? (
          <div className="grid gap-1">
            <span>{detailMessages.providerSessionLabel}</span>
            <button
              aria-label={workstationMessages.selectProviderSessionLabel(
                providerSessionLabel,
                attempt.dispatch_id,
              )}
              aria-pressed={providerSessionSelected}
              className={cn(
                PROVIDER_SESSION_SELECTION_BUTTON_CLASS,
                providerSessionSelected &&
                  "border-af-accent/35 bg-af-accent/10 text-af-accent",
              )}
              onClick={() => onSelectProviderSession(loadableProviderSession)}
              type="button"
            >
              <span className={DASHBOARD_SUPPORTING_TEXT_CLASS}>
                {providerSessionSelected
                  ? workstationMessages.providerSessionSelectedAction
                  : workstationMessages.providerSessionSelectAction}
              </span>
              <code className={DASHBOARD_BODY_CODE_CLASS}>{providerSessionLabel}</code>
            </button>
          </div>
        ) : (
          <div className="grid gap-1">
            <span>{detailMessages.providerSessionLabel}</span>
            <code>{providerSessionLabel}</code>
            <p className={REQUEST_SELECTION_STATUS_CLASS}>
              {workstationMessages.providerSessionSelectionUnavailable}
            </p>
          </div>
        )
      ) : (
        <InferenceAttemptDetail
          code={!providerSessionLogTarget}
          label={detailMessages.providerSessionLabel}
          value={providerSessionLabel}
        />
      )}
      <InferenceAttemptTextSection label={detailMessages.requestBodyLabel} value={attempt.prompt} />
      {attempt.response !== undefined ? (
        <InferenceAttemptTextSection label={detailMessages.responseBodyLabel} value={attempt.response} />
      ) : attempt.outcome ? (
        <p className={DETAIL_COPY_CLASS}>{detailMessages.providerResponseUnavailable}</p>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{detailMessages.awaitingProviderResponse}</p>
      )}
    </article>
  );
}
