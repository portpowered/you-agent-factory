import type { ReactNode } from "react";
import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import { DETAIL_COPY_CLASS } from "../../../components/ui/widget-frame";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { LocalizedTimezoneNote } from "../../../components/ui/localized-timezone-note";
import { getLocalDateTimeDisplay } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import { PROVIDER_SESSION_CARD_CLASS } from "../../current-selection/components/detail-card-shared";
import { useProviderSessionDetail } from "../hooks/use-provider-session-detail";
import type { LoadableProviderSessionRef } from "../lib/provider-session-ref";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";
import { TranscriptSection } from "./provider-session-transcript";

export interface ProviderSessionDetailPanelProps {
  locale?: string;
  selectedProviderSession: LoadableProviderSessionRef | null;
}

type SessionDetail = ProviderSessionDetailResponse;
const PROVIDER_SESSION_SANS_CLASS = "af-provider-session-sans";

export function ProviderSessionDetailPanel({
  locale,
  selectedProviderSession,
}: ProviderSessionDetailPanelProps) {
  if (selectedProviderSession === null) {
    return null;
  }

  return (
    <LoadedProviderSessionDetailPanel
      locale={locale}
      selectedProviderSession={selectedProviderSession}
    />
  );
}

function LoadedProviderSessionDetailPanel({
  locale,
  selectedProviderSession,
}: {
  locale?: string;
  selectedProviderSession: LoadableProviderSessionRef;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const detailState = useProviderSessionDetail(selectedProviderSession);
  const localizedSessionKind = messages.localizeSessionKind(
    selectedProviderSession.kind,
  );
  const sessionLabel = [
    selectedProviderSession.provider,
    localizedSessionKind,
    selectedProviderSession.id,
  ].join(" / ");
  const detail =
    detailState.status === "empty" ||
    detailState.status === "empty-transcript" ||
    detailState.status === "parse-error" ||
    detailState.status === "success"
      ? detailState.sessionDetail
      : null;

  return (
    <section
      aria-label={messages.selectedSessionHeading}
      className={cn(
        "mt-4 grid gap-3 border-t border-af-border pt-4",
        PROVIDER_SESSION_SANS_CLASS,
      )}
    >
      <div className="grid gap-3">
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
          {messages.selectedSessionHeading}
        </h4>
        <LocalizedTimezoneNote
          locale={locale}
          timezoneLabel={messages.localizedTimezoneLabel}
        >
          {messages.localizedTimezoneContext}
        </LocalizedTimezoneNote>
        <div className="grid gap-3">
          <DetailMetric
            label={messages.sessionLabel}
            value={sessionLabel}
            code
          />
          <DetailMetric
            label={messages.sessionStatusLabel}
            value={getSessionStatusText(detailState.status, messages)}
          />
          <DetailMetric
            label={messages.providerLabel}
            value={selectedProviderSession.provider}
            code
          />
          <DetailMetric
            label={messages.kindLabel}
            value={localizedSessionKind}
          />
          <DetailMetric
            label={messages.sessionIdLabel}
            value={selectedProviderSession.id}
            code
          />
          <DetailMetric
            label={messages.dispatchLabel}
            value={selectedProviderSession.dispatchID}
            code
          />
        </div>
      </div>
      {detailState.status === "loading" ? (
        <StatusNotice>{messages.loadingState}</StatusNotice>
      ) : null}
      {detailState.status === "not-found" ? (
        <StatusNotice>{messages.missingState}</StatusNotice>
      ) : null}
      {detailState.status === "error" ? (
        <StatusNotice tone="error">
          {messages.errorPrefix}{" "}
          {detailState.message ?? messages.unavailableState}
        </StatusNotice>
      ) : null}
      {detail ? (
        <>
          <SourceFileSection detail={detail} locale={locale} />
          {detailState.status !== "empty" &&
          detailState.status !== "success" ? (
            <SecondarySection
              description={messages.sessionAnalysisDescription}
              heading={messages.sessionAnalysisHeading}
            >
              <ParseOverview detail={detail} locale={locale} />
              <TokenUsageSection detail={detail} locale={locale} />
            </SecondarySection>
          ) : null}
          {detailState.status === "empty" ? (
            <StatusNotice>{messages.emptyState}</StatusNotice>
          ) : null}
          {detailState.status === "empty-transcript" ? (
            <>
              <StatusNotice>{messages.emptyTranscriptState}</StatusNotice>
              <ParseDiagnosticsSection detail={detail} locale={locale} />
            </>
          ) : null}
          {detailState.status === "parse-error" ? (
            <>
              <StatusNotice tone="error">
                {messages.parseErrorState}
              </StatusNotice>
              <ParseDiagnosticsSection detail={detail} locale={locale} />
            </>
          ) : null}
          {detailState.status === "success" ? (
            <>
              <TranscriptSection detail={detail} locale={locale} />
              <SecondarySection
                description={messages.sessionAnalysisDescription}
                heading={messages.sessionAnalysisHeading}
              >
                <ParseOverview detail={detail} locale={locale} />
                <TokenUsageSection detail={detail} locale={locale} />
                <TurnsSection detail={detail} locale={locale} />
              </SecondarySection>
              <ParseDiagnosticsSection detail={detail} locale={locale} />
            </>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function SourceFileSection({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className="grid gap-2.5">
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.sourceHeading}
      </h5>
      <div className="grid gap-3">
        <DetailMetric
          label={messages.relativePathLabel}
          value={detail.source.relativePath}
        />
        <DetailMetric
          label={messages.sizeBytesLabel}
          value={`${detail.source.sizeBytes.toLocaleString()} ${messages.bytesLabel}`}
        />
        <DetailMetric
          label={messages.modifiedAtLabel}
          value={
            <TimestampMetricValue
              locale={locale}
              timestamp={detail.source.modifiedAt}
              unavailableLabel={messages.unavailableValue}
            />
          }
        />
      </div>
    </section>
  );
}

function StatusNotice({
  children,
  tone = "default",
}: {
  children: ReactNode;
  tone?: "default" | "error";
}) {
  return (
    <p
      className={cn(
        "m-0 rounded-lg border px-3 py-2.5",
        tone === "error"
          ? "border-af-danger-border bg-af-danger-surface text-af-danger-text"
          : "border-af-border bg-af-surface-subtle text-af-text-muted",
        DASHBOARD_BODY_TEXT_CLASS,
      )}
      role={tone === "error" ? "alert" : "status"}
    >
      {children}
    </p>
  );
}

function getSessionStatusText(
  status: ReturnType<typeof useProviderSessionDetail>["status"],
  messages: ReturnType<typeof getProviderSessionDetailMessages>,
) {
  switch (status) {
    case "idle":
      return messages.unavailableState;
    case "loading":
      return messages.loadingState;
    case "not-found":
      return messages.missingState;
    case "error":
      return messages.unavailableState;
    case "empty":
      return messages.emptyState;
    case "empty-transcript":
      return messages.emptyTranscriptState;
    case "parse-error":
      return messages.parseErrorState;
    case "success":
      return messages.readyState;
  }
}

function ParseOverview({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className="grid gap-2.5">
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.parseSummaryHeading}
      </h5>
      <div className="grid gap-3">
        <DetailMetric
          label={messages.eventCountLabel}
          value={detail.parse.eventCount}
        />
        <DetailMetric
          label={messages.lineCountLabel}
          value={detail.parse.lineCount}
        />
        <DetailMetric
          label={messages.malformedLineCountLabel}
          value={detail.parse.malformedLineCount}
        />
        <DetailMetric
          label={messages.unknownEventCountLabel}
          value={detail.parse.unknownEventCount}
        />
      </div>
    </section>
  );
}

function TokenUsageSection({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const tokenUsage = detail.parse.tokenUsage;

  return (
    <section className="grid gap-2.5">
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.tokenUsageHeading}
      </h5>
      {tokenUsage ? (
        <div className="grid gap-3">
          <DetailMetric
            label={messages.inputLabel}
            value={tokenUsage.inputTokens ?? 0}
          />
          <DetailMetric
            label={messages.cachedInputLabel}
            value={tokenUsage.cachedInputTokens ?? 0}
          />
          <DetailMetric
            label={messages.outputLabel}
            value={tokenUsage.outputTokens ?? 0}
          />
          <DetailMetric
            label={messages.reasoningCountLabel}
            value={tokenUsage.reasoningOutputTokens ?? 0}
          />
          <DetailMetric
            label={messages.totalLabel}
            value={tokenUsage.totalTokens ?? 0}
          />
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.tokenUsageUnavailable}</p>
      )}
    </section>
  );
}

function TurnsSection({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className="grid gap-2.5">
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.turnsHeading}
      </h5>
      {detail.parse.turns.length > 0 ? (
        <div className="grid gap-3">
          {detail.parse.turns.map((turn) => (
            <article className={PROVIDER_SESSION_CARD_CLASS} key={turn.index}>
              <div className="grid gap-1">
                <strong>{messages.turnLabel({ index: turn.index })}</strong>
                <div
                  className={cn(
                    "text-af-text-subtle",
                    DASHBOARD_SUPPORTING_TEXT_CLASS,
                  )}
                >
                  <TimestampMetricValue
                    locale={locale}
                    timestamp={turn.startedAt}
                    unavailableLabel={messages.noTimestamp}
                  />
                </div>
              </div>
              <div className="mt-2 grid gap-3">
                <DetailMetric
                  label={messages.eventsLabel}
                  value={turn.eventCount}
                />
                <DetailMetric
                  label={messages.responseItemsLabel}
                  value={turn.responseItemCount}
                />
                <DetailMetric
                  label={messages.functionCallCountLabel}
                  value={turn.functionCallCount}
                />
                <DetailMetric
                  label={messages.reasoningCountLabel}
                  value={turn.reasoningCount}
                />
              </div>
            </article>
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.turnsUnavailable}</p>
      )}
    </section>
  );
}

function ParseDiagnosticsSection({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  if (
    detail.parse.parseErrors.length === 0 &&
    detail.parse.unknownEvents.length === 0
  ) {
    return null;
  }

  return (
    <SecondarySection
      description={messages.maintainerDiagnosticsDescription}
      heading={messages.maintainerDiagnosticsHeading}
    >
      <section className="grid gap-2.5">
        <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
          {messages.parseErrorsHeading}
        </h5>
        <div className="grid gap-3">
          {detail.parse.parseErrors.map((error) => (
            <article
              className={PROVIDER_SESSION_CARD_CLASS}
              key={`parse-error-${error.lineNumber}`}
            >
              <strong>
                {messages.lineLabel({ lineNumber: error.lineNumber })}
              </strong>
              <p className={cn("m-0 mt-1.5", DASHBOARD_BODY_TEXT_CLASS)}>
                {error.message}
              </p>
            </article>
          ))}
          {detail.parse.unknownEvents.map((event) => (
            <article
              className={PROVIDER_SESSION_CARD_CLASS}
              key={`unknown-event-${event.lineNumber}`}
            >
              <strong>
                {messages.unknownEventOnLineLabel({
                  lineNumber: event.lineNumber,
                })}
              </strong>
              <p
                className={cn(
                  "m-0 mt-1.5 text-af-text-subtle",
                  DASHBOARD_SUPPORTING_TEXT_CLASS,
                )}
              >
                {[
                  event.type ? `type=${event.type}` : null,
                  event.payloadType ? `payload=${event.payloadType}` : null,
                ]
                  .filter(Boolean)
                  .join(" / ")}
              </p>
            </article>
          ))}
        </div>
      </section>
    </SecondarySection>
  );
}

function SecondarySection({
  children,
  description,
  heading,
}: {
  children: ReactNode;
  description: string;
  heading: string;
}) {
  return (
    <section className="grid gap-4 rounded-xl border border-af-border bg-af-surface-subtle p-4">
      <div className="grid gap-1">
        <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>{heading}</h5>
        <p
          className={cn(
            "m-0 text-af-text-subtle",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {description}
        </p>
      </div>
      <div className="grid gap-3">{children}</div>
    </section>
  );
}

function DetailMetric({
  code = false,
  label,
  value,
}: {
  code?: boolean;
  label: string;
  value: number | string | ReactNode;
}) {
  const metricValue = code ? (
    <code className={`${DASHBOARD_BODY_CODE_CLASS} [overflow-wrap:anywhere]`}>
      {value}
    </code>
  ) : (
    value
  );
  const wrapperClassName = cn("mt-1", DASHBOARD_BODY_TEXT_CLASS);

  return (
    <div className={PROVIDER_SESSION_CARD_CLASS}>
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      {typeof value === "string" || typeof value === "number" ? (
        <p className={cn("m-0 [overflow-wrap:anywhere]", wrapperClassName)}>
          {metricValue}
        </p>
      ) : (
        <div className={cn("[overflow-wrap:anywhere]", wrapperClassName)}>
          {metricValue}
        </div>
      )}
    </div>
  );
}

function TimestampMetricValue({
  locale,
  timestamp,
  unavailableLabel,
}: {
  locale?: string;
  timestamp?: string | null;
  unavailableLabel: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const timestampDisplay = getLocalDateTimeDisplay(
    timestamp,
    unavailableLabel,
    locale,
  );

  if (!timestampDisplay.rawTimestamp) {
    return timestampDisplay.label;
  }

  return (
    <span className="grid gap-1">
      <span title={timestampDisplay.rawTimestamp}>{timestampDisplay.label}</span>
      <details className="grid gap-1">
        <summary
          className={cn(
            "w-fit cursor-pointer text-af-text-subtle underline decoration-dotted underline-offset-2",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.rawTimestampDetailsLabel}
        </summary>
        <code
          className={cn(
            "w-fit rounded-md border border-af-border bg-af-surface-subtle px-2 py-1",
            DASHBOARD_BODY_CODE_CLASS,
          )}
        >
          {timestampDisplay.rawTimestamp}
        </code>
      </details>
    </span>
  );
}
