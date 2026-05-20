import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import { cn } from "../../lib/cn";
import type { ProviderSessionDetailResponse } from "../../api/provider-session-details";
import { PROVIDER_SESSION_CARD_CLASS } from "./detail-card-shared";
import { getProviderSessionDetailMessages } from "./messages/provider-session-detail";
import type { LoadableProviderSessionRef } from "./provider-session-details";
import { useProviderSessionDetail } from "./use-provider-session-detail";

export interface ProviderSessionDetailPanelProps {
  locale?: string;
  selectedProviderSession: LoadableProviderSessionRef | null;
}

type SessionDetail = ProviderSessionDetailResponse;

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
  const sessionLabel = [
    selectedProviderSession.provider,
    selectedProviderSession.kind,
    selectedProviderSession.id,
  ].join(" / ");

  return (
    <section
      aria-label={messages.selectedSessionHeading}
      className="mt-4 grid gap-3 border-t border-af-overlay/8 pt-4"
    >
      <div className="grid gap-1">
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
          {messages.selectedSessionHeading}
        </h4>
        <p className={cn("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {messages.sessionLabel}
          </span>{" "}
          <code className={DASHBOARD_BODY_CODE_CLASS}>{sessionLabel}</code>
        </p>
      </div>
      {detailState.status === "loading" ? (
        <p className={DETAIL_COPY_CLASS}>{messages.loadingState}</p>
      ) : null}
      {detailState.status === "not-found" ? (
        <p className={DETAIL_COPY_CLASS}>{messages.missingState}</p>
      ) : null}
      {detailState.status === "error" ? (
        <p className={DETAIL_COPY_CLASS}>
          {messages.errorPrefix} {detailState.message ?? messages.unavailableState}
        </p>
      ) : null}
      {detailState.status === "empty" ? (
        <>
          <SourceFileSection
            detail={detailState.sessionDetail}
            locale={locale}
            session={selectedProviderSession}
          />
          <p className={DETAIL_COPY_CLASS}>{messages.emptyState}</p>
        </>
      ) : null}
      {detailState.status === "parse-error" ? (
        <>
          <SourceFileSection
            detail={detailState.sessionDetail}
            locale={locale}
            session={selectedProviderSession}
          />
          <ParseOverview
            detail={detailState.sessionDetail}
            locale={locale}
          />
          <p className={DETAIL_COPY_CLASS}>{messages.parseErrorState}</p>
          <ParseDiagnosticsSection
            detail={detailState.sessionDetail}
            locale={locale}
          />
        </>
      ) : null}
      {detailState.status === "success" ? (
        <>
          <SourceFileSection
            detail={detailState.sessionDetail}
            locale={locale}
            session={selectedProviderSession}
          />
          <ParseOverview
            detail={detailState.sessionDetail}
            locale={locale}
          />
          <TokenUsageSection
            detail={detailState.sessionDetail}
            locale={locale}
          />
          <TurnsSection detail={detailState.sessionDetail} locale={locale} />
          <FunctionCallsSection
            detail={detailState.sessionDetail}
            locale={locale}
          />
          <ReasoningSection
            detail={detailState.sessionDetail}
            locale={locale}
          />
          <ParseDiagnosticsSection
            detail={detailState.sessionDetail}
            locale={locale}
          />
        </>
      ) : null}
    </section>
  );
}

function SourceFileSection({
  detail,
  locale,
  session,
}: {
  detail: SessionDetail;
  locale?: string;
  session: LoadableProviderSessionRef;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className="grid gap-2.5">
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>{messages.sourceHeading}</h5>
      <div className="grid gap-3 md:grid-cols-2">
        <DetailMetric
          label={messages.relativePathLabel}
          value={detail.source.relativePath}
        />
        <DetailMetric label={messages.dispatchLabel} value={session.dispatchID} />
        <DetailMetric
          label={messages.sizeBytesLabel}
          value={`${detail.source.sizeBytes.toLocaleString()} ${messages.bytesLabel}`}
        />
        <DetailMetric
          label={messages.modifiedAtLabel}
          value={detail.source.modifiedAt ?? messages.unavailableValue}
        />
      </div>
    </section>
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
    <section className="grid gap-2.5">
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.parseDiagnosticsHeading}
      </h5>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <DetailMetric
          label={messages.eventCountLabel}
          value={detail.parse.eventCount}
        />
        <DetailMetric label={messages.lineCountLabel} value={detail.parse.lineCount} />
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
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <DetailMetric label={messages.inputLabel} value={tokenUsage.inputTokens ?? 0} />
          <DetailMetric
            label={messages.cachedInputLabel}
            value={tokenUsage.cachedInputTokens ?? 0}
          />
          <DetailMetric label={messages.outputLabel} value={tokenUsage.outputTokens ?? 0} />
          <DetailMetric
            label={messages.reasoningCountLabel}
            value={tokenUsage.reasoningOutputTokens ?? 0}
          />
          <DetailMetric label={messages.totalLabel} value={tokenUsage.totalTokens ?? 0} />
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
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>{messages.turnsHeading}</h5>
      {detail.parse.turns.length > 0 ? (
        <div className="grid gap-3 md:grid-cols-2">
          {detail.parse.turns.map((turn) => (
            <article className={PROVIDER_SESSION_CARD_CLASS} key={turn.index}>
              <div className="grid gap-1">
                <strong>{messages.turnLabel({ index: turn.index })}</strong>
                <p
                  className={cn(
                    "m-0 text-af-ink/62",
                    DASHBOARD_SUPPORTING_TEXT_CLASS,
                  )}
                >
                  {turn.startedAt ?? messages.noTimestamp}
                </p>
              </div>
              <div className="mt-2 grid gap-3 sm:grid-cols-2">
                <DetailMetric label={messages.eventsLabel} value={turn.eventCount} />
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

function FunctionCallsSection({
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
        {messages.functionCallsHeading}
      </h5>
      {detail.parse.functionCalls.length > 0 ? (
        <div className="grid gap-3">
          {detail.parse.functionCalls.map((call) => (
            <article
              className={PROVIDER_SESSION_CARD_CLASS}
              key={`${call.order}-${call.callId ?? call.name ?? call.type}`}
            >
              <div className="grid gap-1 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
                <div className="grid gap-1">
                  <strong>{call.name ?? call.type}</strong>
                  <p
                    className={cn(
                      "m-0 text-af-ink/62",
                      DASHBOARD_SUPPORTING_TEXT_CLASS,
                    )}
                  >
                    {messages.orderLabel({
                      order: call.order,
                      turnIndex: call.turnIndex,
                    })}
                  </p>
                </div>
                {call.status ? (
                  <span
                    className={cn(
                      "inline-flex w-fit rounded-full border border-af-overlay/12 bg-af-overlay/6 px-2 py-0.5",
                      DASHBOARD_SUPPORTING_TEXT_CLASS,
                    )}
                  >
                    {call.status}
                  </span>
                ) : null}
              </div>
              <div className="mt-2 grid gap-3 md:grid-cols-2">
                <DetailMetric label={messages.typeLabel} value={call.type} />
                <DetailMetric
                  label={messages.callIdLabel}
                  value={call.callId ?? messages.unavailableValue}
                />
                {call.arguments ? (
                  <CodeBlockMetric label={messages.argumentsLabel} value={call.arguments} />
                ) : null}
                {call.output ? (
                  <CodeBlockMetric label={messages.outputLabel} value={call.output} />
                ) : null}
              </div>
            </article>
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.functionCallsUnavailable}</p>
      )}
    </section>
  );
}

function ReasoningSection({
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
        {messages.reasoningHeading}
      </h5>
      {detail.parse.reasoning.length > 0 ? (
        <div className="grid gap-3">
          {detail.parse.reasoning.map((entry) => (
            <article
              className={PROVIDER_SESSION_CARD_CLASS}
              key={`${entry.order}-${entry.sourceType}`}
            >
              <div className="grid gap-1">
                <strong>{entry.sourceType}</strong>
                <p
                  className={cn(
                    "m-0 text-af-ink/62",
                    DASHBOARD_SUPPORTING_TEXT_CLASS,
                  )}
                >
                  {messages.orderLabel({
                    order: entry.order,
                    turnIndex: entry.turnIndex,
                  })}
                </p>
              </div>
              {entry.summary ? (
                <p className={cn("m-0 mt-2", DASHBOARD_BODY_TEXT_CLASS)}>
                  {entry.summary}
                </p>
              ) : null}
              {entry.text ? (
                <pre
                  className={cn(
                    "m-0 mt-2 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-3 [overflow-wrap:anywhere]",
                    DASHBOARD_BODY_CODE_CLASS,
                  )}
                >
                  {entry.text}
                </pre>
              ) : null}
              {entry.encrypted ? (
                <p
                  className={cn(
                    "m-0 mt-2 text-af-ink/62",
                    DASHBOARD_SUPPORTING_TEXT_CLASS,
                  )}
                >
                  {messages.encryptedReasoningOnly}
                </p>
              ) : null}
            </article>
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.reasoningUnavailable}</p>
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
            <strong>{messages.lineLabel({ lineNumber: error.lineNumber })}</strong>
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
              {messages.unknownEventOnLineLabel({ lineNumber: event.lineNumber })}
            </strong>
            <p
              className={cn(
                "m-0 mt-1.5 text-af-ink/62",
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
  );
}

function DetailMetric({
  label,
  value,
}: {
  label: string;
  value: number | string;
}) {
  return (
    <div className={PROVIDER_SESSION_CARD_CLASS}>
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <p className={cn("m-0 mt-1", DASHBOARD_BODY_TEXT_CLASS)}>
        {value}
      </p>
    </div>
  );
}

function CodeBlockMetric({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div className="grid gap-1">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <pre
        className={cn(
          "m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-3 [overflow-wrap:anywhere]",
          DASHBOARD_BODY_CODE_CLASS,
        )}
      >
        {value}
      </pre>
    </div>
  );
}
