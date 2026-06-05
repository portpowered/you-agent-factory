import { useId, useState } from "react";
import type { DashboardInferenceAttempt } from "../../../../api/dashboard/types";
import {
  DashboardCode,
  DashboardText,
  ExpandablePanelTrigger,
  surfacePanelVariants,
} from "../../../../components/ui";
import {
  formatDurationMillis,
  formatLocalDateTime,
  getLocalDateTimeDisplay,
  getProviderSessionLogTarget,
} from "../../../../components/ui/formatters";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import {
  getLoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "../../../provider-session-detail/lib/provider-session-ref";
import { CurrentSelectionExpandableSection } from "../../base/components/current-selection-expandable-section";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
  useCurrentSelectionWorkstationDetailMessages,
} from "../../base/components/current-selection-locale";
import { CurrentSelectionExecutionPill } from "../../base/components/current-selection-pill";
import { CurrentSelectionSelectableButton } from "../../base/components/current-selection-selectable-button";
import { normalizeDetailText } from "../../base/components/detail-card-shared";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionLabel,
  CurrentSelectionSupportingText,
} from "../../base/public";
import type { InferenceAttemptCardProps } from "../lib/detail-card-types";
import { InferenceAttemptDetail } from "./inference-attempt-detail";
import { InferenceAttemptTextSection } from "./inference-attempt-text-section";

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
  const timingSummary = getAttemptTimingSummary(
    attempt,
    detailMessages,
    locale,
  );

  return (
    <article
      aria-label={detailMessages.attemptAriaLabel(attempt.attempt)}
      className={surfacePanelVariants({
        className: "grid min-w-0 gap-2.5 p-3.5",
        radius: "lg",
      })}
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
  const panelId = useId();
  const labelId = `${panelId}-label`;

  if (!value) {
    return null;
  }

  return (
    <CurrentSelectionExpandableSection
      className="mt-0"
      contentId={panelId}
      headingId={labelId}
      title={label}
      toggleLabel={(expanded) => (expanded ? collapseAction : expandAction)}
    >
      <InferenceAttemptTextSection label={label} value={value} />
    </CurrentSelectionExpandableSection>
  );
}

function AttemptMetadataDetails({
  attempt,
}: {
  attempt: DashboardInferenceAttempt;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();
  const locale = useCurrentSelectionLocale();
  const provider =
    attempt.diagnostics?.provider?.provider ??
    attempt.provider_session?.provider;
  const model = attempt.diagnostics?.provider?.model;
  const requestTime = getLocalDateTimeDisplay(
    attempt.request_time,
    detailMessages.timestampUnavailable,
    locale,
  );
  const responseTime = getLocalDateTimeDisplay(
    attempt.response_time,
    detailMessages.timestampUnavailable,
    locale,
  );

  return (
    <CurrentSelectionDescriptionList>
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
      <InferenceAttemptDetail
        code
        label={detailMessages.modelLabel}
        value={model}
      />
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
      <InferenceAttemptDetail
        label={detailMessages.requestTimeLabel}
        rawValue={requestTime.rawTimestamp ?? undefined}
        value={requestTime.label}
      />
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
            ? formatDurationMillis(attempt.duration_millis, locale)
            : undefined
        }
      />
      <InferenceAttemptDetail
        label={detailMessages.responseTimeLabel}
        rawValue={responseTime.rawTimestamp ?? undefined}
        value={responseTime.label}
      />
      <InferenceAttemptDetail
        label={detailMessages.exitCodeLabel}
        value={attempt.exit_code}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.errorClassLabel}
        value={attempt.error_class}
      />
    </CurrentSelectionDescriptionList>
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
        <CurrentSelectionLabel>
          {detailMessages.providerSessionLabel}
        </CurrentSelectionLabel>
        <CurrentSelectionSelectableButton
          aria-label={workstationMessages.selectProviderSessionLabel(
            state.providerSessionLabel,
            attempt.dispatch_id,
          )}
          onClick={() => onSelectProviderSession(loadableProviderSession)}
          selected={state.providerSessionSelected}
          variant="card"
        >
          <DashboardText as="span" variant="supporting">
            {state.providerSessionSelected
              ? workstationMessages.providerSessionSelectedAction
              : workstationMessages.providerSessionSelectAction}
          </DashboardText>
          <DashboardCode>
            {state.providerSessionLabel}
          </DashboardCode>
        </CurrentSelectionSelectableButton>
      </div>
    );
  }

  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>
        {detailMessages.providerSessionLabel}
      </CurrentSelectionLabel>
      <DashboardCode>{state.providerSessionLabel}</DashboardCode>
      <CurrentSelectionSupportingText tone="status">
        {workstationMessages.providerSessionSelectionUnavailable}
      </CurrentSelectionSupportingText>
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
    <DetailCopy>{detailMessages.providerResponseUnavailable}</DetailCopy>
  ) : (
    <DetailCopy>{detailMessages.awaitingProviderResponse}</DetailCopy>
  );
}

function useAttemptProviderSessionState({
  attempt,
  selectedProviderSessionKey,
}: {
  attempt: DashboardInferenceAttempt;
  selectedProviderSessionKey?: string | null;
}) {
  const workstationMessages = useCurrentSelectionWorkstationDetailMessages();
  const providerSessionLogTarget = getProviderSessionLogTarget(
    attempt.provider_session,
    attempt.request_time,
  );
  const loadableProviderSession = getLoadableProviderSessionRef({
    dispatch_id: attempt.dispatch_id,
    provider_session: attempt.provider_session,
  });
  const providerSessionLabel = attempt.provider_session
    ? formatLocalizedProviderSessionLabel(
        attempt.provider_session,
        workstationMessages,
      )
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

function formatLocalizedProviderSessionLabel(
  session: DashboardInferenceAttempt["provider_session"],
  workstationMessages: ReturnType<
    typeof useCurrentSelectionWorkstationDetailMessages
  >,
): string {
  if (!session?.id) {
    return workstationMessages.unavailableValue;
  }

  const localizedKind = localizeProviderSessionKind(
    session.kind,
    workstationMessages,
  );
  const parts = [session.provider, localizedKind].filter(
    (value): value is string => value !== undefined && value !== "",
  );

  if (parts.length === 0) {
    return session.id;
  }

  return `${parts.join(" / ")} / ${session.id}`;
}

function localizeProviderSessionKind(
  kind: string | undefined,
  workstationMessages: ReturnType<
    typeof useCurrentSelectionWorkstationDetailMessages
  >,
): string | undefined {
  const normalizedKind = kind?.trim();
  if (!normalizedKind) {
    return undefined;
  }

  return workstationMessages.localizeProviderSessionKind(normalizedKind);
}

function getAttemptTimingSummary(
  attempt: DashboardInferenceAttempt,
  detailMessages: ReturnType<typeof useCurrentSelectionDetailMessages>,
  locale?: string | null,
): string | undefined {
  if (attempt.duration_millis !== undefined) {
    return `${detailMessages.elapsedTimeLabel}: ${formatDurationMillis(
      attempt.duration_millis,
      locale,
    )}`;
  }

  if (attempt.response_time) {
    return `${detailMessages.responseTimeLabel}: ${formatLocalDateTime(
      attempt.response_time,
      detailMessages.timestampUnavailable,
      locale,
    )}`;
  }

  return undefined;
}
