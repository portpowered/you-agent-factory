import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import { Heading, Label, Text } from "@you-agent-factory/components/primitives";
import { AlertPanel } from "../../../components/ui/alert-panel";
import { cn } from "../../../lib/cn";
import { CurrentSelectionHistoryCard } from "../../current-selection/history/components/current-selection-history-card";
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
  const tokenUsage = detail?.parse.tokenUsage;
  const selectedSessionResetKey = [
    selectedProviderSession.provider,
    selectedProviderSession.kind,
    selectedProviderSession.id,
  ].join(":");

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
              {
                label: messages.inputTokensLabel,
                value: tokenUsage?.inputTokens ?? messages.unavailableValue,
              },
              {
                label: messages.outputTokensLabel,
                value: tokenUsage?.outputTokens ?? messages.unavailableValue,
              },
              {
                label: messages.cachedTokensLabel,
                value:
                  tokenUsage?.cachedInputTokens ?? messages.unavailableValue,
              },
              {
                label: messages.sourceHeading,
                value: detail?.source.relativePath ?? messages.unavailableValue,
              },
            ]}
          />
        }
        resetKey={selectedSessionResetKey}
      >
        {detail ? (
          <SelectedSessionDetails detail={detail} locale={locale} />
        ) : null}
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
                defaultExpanded
                heading={messages.transcriptHeading}
                locale={locale}
                preview={
                  <TranscriptSectionPreview detail={detail} locale={locale} />
                }
                resetKey={selectedSessionResetKey}
              >
                <TranscriptSection
                  detail={detail}
                  key={selectedSessionResetKey}
                  locale={locale}
                  showHeading={false}
                />
              </ProviderSessionExpandableSection>
              <ParseDiagnosticsSection detail={detail} locale={locale} />
            </>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function SelectedSessionDetails({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <div className="grid gap-6">
      <DetailMetric
        label={messages.sessionIdLabel}
        value={detail.providerSession.id}
      />
      <section className="grid gap-3">
        <Heading as="h4">{messages.sourceMetadataHeading}</Heading>
        <div className="grid gap-4">
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
      <section className="grid gap-3">
        <Heading as="h4">{messages.sessionAnalysisHeading}</Heading>
        <ParseOverview detail={detail} locale={locale} />
        <TokenUsageSection detail={detail} locale={locale} />
        <TurnsSection detail={detail} locale={locale} />
      </section>
    </div>
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
    <AlertPanel
      radius="lg"
      role={tone === "error" ? "alert" : "status"}
      tone={tone === "error" ? "danger" : "info"}
    >
      {children}
    </AlertPanel>
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
          <WidgetDetailCopy>{messages.tokenUsageUnavailable}</WidgetDetailCopy>
        )
      }
    >
      {tokenUsage ? (
        <div className="grid gap-4">
          <DetailMetric
            label={messages.reasoningCountLabel}
            value={
              tokenUsage.reasoningOutputTokens ?? messages.unavailableValue
            }
          />
          <DetailMetric
            label={messages.totalLabel}
            value={tokenUsage.totalTokens ?? messages.unavailableValue}
          />
        </div>
      ) : (
        <WidgetDetailCopy>{messages.tokenUsageUnavailable}</WidgetDetailCopy>
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
          <WidgetDetailCopy>{messages.turnsUnavailable}</WidgetDetailCopy>
        )
      }
    >
      {detail.parse.turns.length > 0 ? (
        <div className="grid gap-4">
          {detail.parse.turns.map((turn) => (
            <article className="grid gap-3 py-1.5" key={turn.index}>
              <div className="grid gap-1">
                <Label>{messages.turnLabel({ index: turn.index })}</Label>
                <Text
                  as="div"
                  className="text-on-surface-subtle"
                  variant="supporting"
                >
                  <TimestampMetricValue
                    locale={locale}
                    timestamp={turn.startedAt}
                    unavailableLabel={messages.noTimestamp}
                  />
                </Text>
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
        <WidgetDetailCopy>{messages.turnsUnavailable}</WidgetDetailCopy>
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
        <Heading as="h5">{messages.parseErrorsHeading}</Heading>
        <div className="grid gap-3">
          {detail.parse.parseErrors.map((error) => (
            <CurrentSelectionHistoryCard
              key={`parse-error-${error.lineNumber}`}
            >
              <strong>
                {messages.lineLabel({ lineNumber: error.lineNumber })}
              </strong>
              <Text className="m-0 mt-1.5">{error.message}</Text>
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
              <Text
                className="m-0 mt-1.5 text-on-surface-subtle"
                variant="supporting"
              >
                {[
                  event.type ? `type=${event.type}` : null,
                  event.payloadType ? `payload=${event.payloadType}` : null,
                ]
                  .filter(Boolean)
                  .join(" / ")}
              </Text>
            </CurrentSelectionHistoryCard>
          ))}
        </div>
      </section>
    </ProviderSessionExpandableSection>
  );
}
