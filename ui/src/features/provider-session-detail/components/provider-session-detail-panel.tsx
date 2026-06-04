import type { ReactNode } from "react";
import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../components/ui/widget-frame";
import { cn } from "../../../lib/cn";
import { CurrentSelectionHistoryCard } from "../../current-selection/base/public";
import { useProviderSessionDetail } from "../hooks/use-provider-session-detail";
import type { LoadableProviderSessionRef } from "../lib/provider-session-ref";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";
import {
  DetailMetric,
  TimestampMetricValue,
} from "./provider-session-detail-metrics";
import {
  ProviderSessionExpandableSection,
  SectionMetricPreview,
  TranscriptSectionPreview,
} from "./provider-session-detail-section";
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
      className={cn("grid gap-6", PROVIDER_SESSION_SANS_CLASS)}
    >
      <ProviderSessionExpandableSection
        heading={messages.selectedSessionHeading}
        locale={locale}
        preview={
          <SectionMetricPreview
            items={[
              {
                label: messages.sessionIdLabel,
                value: selectedProviderSession.id,
              },
            ]}
          />
        }
      >
        <div className="grid gap-4">
          <DetailMetric
            label={messages.sessionIdLabel}
            value={selectedProviderSession.id}
          />
        </div>
      </ProviderSessionExpandableSection>
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
            <SessionAnalysisSection detail={detail} locale={locale}>
              <ParseOverview detail={detail} locale={locale} />
              <TokenUsageSection detail={detail} locale={locale} />
            </SessionAnalysisSection>
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
              <ProviderSessionExpandableSection
                heading={messages.transcriptHeading}
                locale={locale}
                preview={
                  <TranscriptSectionPreview detail={detail} locale={locale} />
                }
              >
                <TranscriptSection
                  detail={detail}
                  locale={locale}
                  showHeading={false}
                />
              </ProviderSessionExpandableSection>
              <SessionAnalysisSection detail={detail} locale={locale}>
                <ParseOverview detail={detail} locale={locale} />
                <TokenUsageSection detail={detail} locale={locale} />
                <TurnsSection detail={detail} locale={locale} />
              </SessionAnalysisSection>
              <ParseDiagnosticsSection detail={detail} locale={locale} />
            </>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function SessionAnalysisSection({
  children,
  detail,
  locale,
}: {
  children: ReactNode;
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <ProviderSessionExpandableSection
      heading={messages.sessionAnalysisHeading}
      locale={locale}
      preview={
        <SectionMetricPreview
          items={[
            {
              label: messages.eventCountLabel,
              value: detail.parse.eventCount,
            },
            {
              label: messages.turnsHeading,
              value: detail.parse.turns.length,
            },
            {
              label: messages.totalLabel,
              value: detail.parse.tokenUsage?.totalTokens ?? 0,
            },
          ]}
        />
      }
    >
      {children}
    </ProviderSessionExpandableSection>
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
    <ProviderSessionExpandableSection
      heading={messages.sourceHeading}
      locale={locale}
      preview={
        <SectionMetricPreview
          items={[
            {
              label: messages.relativePathLabel,
              value: detail.source.relativePath,
            },
          ]}
        />
      }
    >
      <div className="grid gap-4">
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
    </ProviderSessionExpandableSection>
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
          ? "border-af-danger-border bg-error-container text-on-error-container"
          : "border-outline bg-surface-container-low text-on-surface-variant",
        DASHBOARD_BODY_TEXT_CLASS,
      )}
      role={tone === "error" ? "alert" : "status"}
    >
      {children}
    </p>
  );
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
    <ProviderSessionExpandableSection
      heading={messages.parseSummaryHeading}
      locale={locale}
      preview={
        <SectionMetricPreview
          items={[
            {
              label: messages.eventCountLabel,
              value: detail.parse.eventCount,
            },
            {
              label: messages.lineCountLabel,
              value: detail.parse.lineCount,
            },
          ]}
        />
      }
    >
      <div className="grid gap-4">
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
    </ProviderSessionExpandableSection>
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
    <ProviderSessionExpandableSection
      heading={messages.tokenUsageHeading}
      locale={locale}
      preview={
        tokenUsage ? (
          <SectionMetricPreview
            items={[
              {
                label: messages.inputLabel,
                value: tokenUsage.inputTokens ?? 0,
              },
              {
                label: messages.outputLabel,
                value: tokenUsage.outputTokens ?? 0,
              },
              {
                label: messages.totalLabel,
                value: tokenUsage.totalTokens ?? 0,
              },
            ]}
          />
        ) : (
          <p className={DETAIL_COPY_CLASS}>{messages.tokenUsageUnavailable}</p>
        )
      }
    >
      {tokenUsage ? (
        <div className="grid gap-4">
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
    </ProviderSessionExpandableSection>
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
    <ProviderSessionExpandableSection
      heading={messages.turnsHeading}
      locale={locale}
      preview={
        detail.parse.turns.length > 0 ? (
          <SectionMetricPreview
            items={[
              {
                label: messages.turnsHeading,
                value: detail.parse.turns.length,
              },
              {
                label: messages.eventsLabel,
                value: detail.parse.turns.reduce(
                  (total, turn) => total + turn.eventCount,
                  0,
                ),
              },
            ]}
          />
        ) : (
          <p className={DETAIL_COPY_CLASS}>{messages.turnsUnavailable}</p>
        )
      }
    >
      {detail.parse.turns.length > 0 ? (
        <div className="grid gap-4">
          {detail.parse.turns.map((turn) => (
            <article className="grid gap-3 py-1.5" key={turn.index}>
              <div className="grid gap-1">
                <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
                  {messages.turnLabel({ index: turn.index })}
                </span>
                <div
                  className={cn(
                    "text-on-surface-subtle",
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
              <div className="grid gap-4">
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
    </ProviderSessionExpandableSection>
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
    <ProviderSessionExpandableSection
      heading={messages.maintainerDiagnosticsHeading}
      locale={locale}
      preview={
        <SectionMetricPreview
          items={[
            {
              label: messages.malformedLineCountLabel,
              value: detail.parse.parseErrors.length,
            },
            {
              label: messages.unknownEventCountLabel,
              value: detail.parse.unknownEvents.length,
            },
          ]}
        />
      }
    >
      <section className="grid gap-3">
        <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
          {messages.parseErrorsHeading}
        </h5>
        <div className="grid gap-3">
          {detail.parse.parseErrors.map((error) => (
            <CurrentSelectionHistoryCard
              key={`parse-error-${error.lineNumber}`}
            >
              <strong>
                {messages.lineLabel({ lineNumber: error.lineNumber })}
              </strong>
              <p className={cn("m-0 mt-1.5", DASHBOARD_BODY_TEXT_CLASS)}>
                {error.message}
              </p>
            </CurrentSelectionHistoryCard>
          ))}
          {detail.parse.unknownEvents.map((event) => (
            <CurrentSelectionHistoryCard
              key={`unknown-event-${event.lineNumber}`}
            >
              <strong>
                {messages.unknownEventOnLineLabel({
                  lineNumber: event.lineNumber,
                })}
              </strong>
              <p
                className={cn(
                  "m-0 mt-1.5 text-on-surface-subtle",
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
            </CurrentSelectionHistoryCard>
          ))}
        </div>
      </section>
    </ProviderSessionExpandableSection>
  );
}
