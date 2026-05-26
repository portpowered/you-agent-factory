import { useId, useState } from "react";
import type { DashboardInferenceAttempt } from "../../../api/dashboard/types";
import {
  formatLocalDateTime,
  formatDurationMillis,
  formatProviderSession,
  getProviderSessionLogTarget,
} from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../components/dashboard/widget-board";
import {
  CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
  EXECUTION_PILL_CLASS,
  HISTORY_HEADER_CLASS,
  HISTORY_TOGGLE_CLASS,
  INFERENCE_ATTEMPT_CARD_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  InferenceAttemptTextSection,
  PROVIDER_SESSION_SELECTION_BUTTON_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
  normalizeDetailText,
} from "./detail-card-shared";
import type { InferenceAttemptCardProps } from "./detail-card-types";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionOperationalEnumMessages,
  useCurrentSelectionWorkstationDetailMessages,
} from "./current-selection-locale";
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "../../provider-session-detail/lib/provider-session-ref";

export function InferenceAttemptCard({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: InferenceAttemptCardProps) {
  const [expanded, setExpanded] = useState(false);
  const attemptPanelId = useId();
  const summaryHeadingId = `${attemptPanelId}-heading`;
  const detailMessages = useCurrentSelectionDetailMessages();
  const timingSummary = getAttemptTimingSummary(attempt, detailMessages);

  return (
    <article
      aria-label={detailMessages.attemptAriaLabel(attempt.attempt)}
      className={INFERENCE_ATTEMPT_CARD_CLASS}
    >
      <AttemptSummaryHeader
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

function AttemptSummaryHeader({
  attempt,
  expanded,
  headingId,
  onToggle,
  panelId,
  timingSummary,
}: {
  attempt: DashboardInferenceAttempt;
  expanded: boolean;
  headingId: string;
  onToggle: () => void;
  panelId: string;
  timingSummary: string | undefined;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();

  return (
    <div className={HISTORY_HEADER_CLASS}>
      <div className="grid min-w-0 gap-1">
        <div className="flex items-start justify-between gap-3">
          <strong id={headingId}>
            {detailMessages.attemptTitle(attempt.attempt)}
          </strong>
          <span className={EXECUTION_PILL_CLASS}>
            {attempt.outcome
              ? enumMessages.localizeOutcome(attempt.outcome)
              : enumMessages.localizeOutcome("PENDING")}
          </span>
        </div>
        {timingSummary ? (
          <p className={REQUEST_SELECTION_STATUS_CLASS}>{timingSummary}</p>
        ) : null}
      </div>
      <button
        aria-controls={panelId}
        aria-expanded={expanded}
        className={HISTORY_TOGGLE_CLASS}
        onClick={onToggle}
        type="button"
      >
        {expanded
          ? detailMessages.collapseAttemptAction(attempt.attempt)
          : detailMessages.expandAttemptAction(attempt.attempt)}
      </button>
    </div>
  );
}

function AttemptExpandedContent({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: InferenceAttemptCardProps) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const providerSessionState = useAttemptProviderSessionState({
    attempt,
    selectedProviderSessionKey,
  });

  return (
    <>
      <AttemptMetadataDetails attempt={attempt} />
      <AttemptProviderSessionDetails
        attempt={attempt}
        onSelectProviderSession={onSelectProviderSession}
        state={providerSessionState}
      />
      <AttemptTextBodyDisclosure
        expandAction={detailMessages.expandRequestBodyAction}
        collapseAction={detailMessages.collapseRequestBodyAction}
        label={detailMessages.requestBodyLabel}
        value={normalizeDetailText(attempt.prompt)}
      />
      <AttemptResponseDetails attempt={attempt} />
    </>
  );
}

function AttemptTextBodyDisclosure({
  collapseAction,
  expandAction,
  label,
  value,
}: {
  collapseAction: string;
  expandAction: string;
  label: string;
  value?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const panelId = useId();
  const labelId = `${panelId}-label`;

  if (!value) {
    return null;
  }

  return (
    <div className="grid gap-1.5">
      <div className={HISTORY_HEADER_CLASS}>
        <strong id={labelId}>{label}</strong>
        <button
          aria-controls={panelId}
          aria-expanded={expanded}
          className={HISTORY_TOGGLE_CLASS}
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? collapseAction : expandAction}
        </button>
      </div>
      {expanded ? (
        <div id={panelId}>
          <InferenceAttemptTextSection label={label} value={value} />
        </div>
      ) : null}
    </div>
  );
}

function AttemptMetadataDetails({
  attempt,
}: {
  attempt: DashboardInferenceAttempt;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();
  const provider =
    attempt.diagnostics?.provider?.provider ?? attempt.provider_session?.provider;
  const model = attempt.diagnostics?.provider?.model;
  const requestTime = formatLocalDateTime(
    attempt.request_time,
    detailMessages.timestampUnavailable,
  );
  const responseTime = formatLocalDateTime(
    attempt.response_time,
    detailMessages.timestampUnavailable,
  );

  return (
    <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
      <InferenceAttemptDetail
        code
        label={detailMessages.inferenceRequestIdLabel}
        value={attempt.inference_request_id}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.providerLabel}
        value={provider}
      />
      <InferenceAttemptDetail code label={detailMessages.modelLabel} value={model} />
      <InferenceAttemptDetail
        code
        label={detailMessages.workingDirectoryLabel}
        value={attempt.working_directory}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.worktreeLabel}
        value={attempt.worktree}
      />
      <InferenceAttemptDetail label={detailMessages.requestTimeLabel} value={requestTime} />
      <InferenceAttemptDetail
        code
        label={detailMessages.outcomeLabel}
        value={
          attempt.outcome
            ? enumMessages.localizeOutcome(attempt.outcome)
            : undefined
        }
      />
      <InferenceAttemptDetail
        label={detailMessages.elapsedTimeLabel}
        value={
          attempt.duration_millis !== undefined
            ? formatDurationMillis(attempt.duration_millis)
            : undefined
        }
      />
      <InferenceAttemptDetail label={detailMessages.responseTimeLabel} value={responseTime} />
      <InferenceAttemptDetail
        label={detailMessages.exitCodeLabel}
        value={attempt.exit_code}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.errorClassLabel}
        value={attempt.error_class}
      />
    </dl>
  );
}

function AttemptProviderSessionDetails({
  attempt,
  onSelectProviderSession,
  state,
}: {
  attempt: DashboardInferenceAttempt;
  onSelectProviderSession?: InferenceAttemptCardProps["onSelectProviderSession"];
  state: ReturnType<typeof useAttemptProviderSessionState>;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const workstationMessages = useCurrentSelectionWorkstationDetailMessages();

  if (!state.providerSessionLabel) {
    return (
      <InferenceAttemptDetail
        code={!state.providerSessionLogTarget}
        label={detailMessages.providerSessionLabel}
        value={state.providerSessionLabel}
      />
    );
  }

  if (state.loadableProviderSession && onSelectProviderSession) {
    const loadableProviderSession = state.loadableProviderSession;

    return (
      <div className="grid gap-1">
        <span>{detailMessages.providerSessionLabel}</span>
        <button
          aria-label={workstationMessages.selectProviderSessionLabel(
            state.providerSessionLabel,
            attempt.dispatch_id,
          )}
          aria-pressed={state.providerSessionSelected}
          className={cn(
            PROVIDER_SESSION_SELECTION_BUTTON_CLASS,
            state.providerSessionSelected &&
              CURRENT_SELECTION_ACCENT_SURFACE_CLASS,
          )}
          onClick={() => onSelectProviderSession(loadableProviderSession)}
          type="button"
        >
          <span className={DASHBOARD_SUPPORTING_TEXT_CLASS}>
            {state.providerSessionSelected
              ? workstationMessages.providerSessionSelectedAction
              : workstationMessages.providerSessionSelectAction}
          </span>
          <code className={DASHBOARD_BODY_CODE_CLASS}>
            {state.providerSessionLabel}
          </code>
        </button>
      </div>
    );
  }

  return (
    <div className="grid gap-1">
      <span>{detailMessages.providerSessionLabel}</span>
      <code>{state.providerSessionLabel}</code>
      <p className={REQUEST_SELECTION_STATUS_CLASS}>
        {workstationMessages.providerSessionSelectionUnavailable}
      </p>
    </div>
  );
}

function AttemptResponseDetails({
  attempt,
}: {
  attempt: DashboardInferenceAttempt;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const response = normalizeDetailText(attempt.response);

  if (response) {
    return (
      <AttemptTextBodyDisclosure
        collapseAction={detailMessages.collapseResponseBodyAction}
        expandAction={detailMessages.expandResponseBodyAction}
        label={detailMessages.responseBodyLabel}
        value={response}
      />
    );
  }

  return attempt.outcome ? (
    <p className={DETAIL_COPY_CLASS}>
      {detailMessages.providerResponseUnavailable}
    </p>
  ) : (
    <p className={DETAIL_COPY_CLASS}>
      {detailMessages.awaitingProviderResponse}
    </p>
  );
}

function useAttemptProviderSessionState({
  attempt,
  selectedProviderSessionKey,
}: {
  attempt: DashboardInferenceAttempt;
  selectedProviderSessionKey?: string | null;
}) {
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

  return {
    loadableProviderSession,
    providerSessionLabel,
    providerSessionLogTarget,
    providerSessionSelected,
  };
}

function getAttemptTimingSummary(
  attempt: DashboardInferenceAttempt,
  detailMessages: ReturnType<typeof useCurrentSelectionDetailMessages>,
): string | undefined {
  if (attempt.duration_millis !== undefined) {
    return `${detailMessages.elapsedTimeLabel}: ${formatDurationMillis(
      attempt.duration_millis,
    )}`;
  }

  if (attempt.response_time) {
    return `${detailMessages.responseTimeLabel}: ${formatLocalDateTime(
      attempt.response_time,
      detailMessages.timestampUnavailable,
    )}`;
  }

  return undefined;
}
