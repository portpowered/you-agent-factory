import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { getLocalDateTimeDisplay } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import {
  AuthoredBodyText,
  PROVIDER_SESSION_CARD_CLASS,
} from "../../current-selection/components/detail-card-shared";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";
import { FriendlyExecCommandOutput } from "./exec-command-output";
import { CodePanel, ExpandableCodeBlock } from "./transcript-code-block";

type SessionDetail = ProviderSessionDetailResponse;
type TranscriptEntry = SessionDetail["transcript"][number];

const TRANSCRIPT_ENTRY_CLASS_NAMES: Record<TranscriptEntry["type"], string> = {
  assistant_message: "border-af-border",
  reasoning: "border-af-border",
  system_event: "border-af-border",
  tool_call: "border-af-border",
  tool_output: "border-af-border",
  user_message: "border-af-border",
};

export function TranscriptSection({
  className,
  detail,
  locale,
}: {
  className?: string;
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className={cn("grid gap-3 rounded-xl border p-4", className)}>
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.transcriptHeading}
      </h5>
      <div className="grid gap-3">
        {detail.transcript.map((entry) => (
          <TranscriptEntryCard
            entry={entry}
            key={entry.order}
            locale={locale}
          />
        ))}
      </div>
    </section>
  );
}

export function EncryptedReasoningNotice({
  className,
  locale,
}: {
  className?: string;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <div
      className={cn(
        "grid gap-2 rounded-lg border border-af-info-border bg-af-info-surface p-3",
        className,
      )}
    >
      <span
        className={cn(
          "inline-flex w-fit rounded-full border border-af-info-border bg-af-info-surface px-2 py-0.5 text-af-info-text",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
      >
        {messages.encryptedReasoningStateLabel}
      </span>
      <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
        {messages.encryptedReasoningDescription}
      </p>
    </div>
  );
}

function TranscriptEntryCard({
  entry,
  locale,
}: {
  entry: TranscriptEntry;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const entryLabel = getTranscriptEntryLabel(entry, messages);
  const metadata = [
    messages.orderLabel({
      order: entry.order,
      turnIndex: entry.turnIndex,
    }),
    entry.lineNumber
      ? messages.transcriptLineNumberLabel({ lineNumber: entry.lineNumber })
      : null,
  ].filter(Boolean);
  const timestampState = getTranscriptTimestampState(entry.timestamp, locale);

  return (
    <article
      className={cn(
        PROVIDER_SESSION_CARD_CLASS,
        "grid gap-3",
        getTranscriptEntryClassName(entry.type),
      )}
    >
      <div className="grid gap-2">
        <div className="grid gap-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <strong>{getTranscriptEntryTitle(entry, entryLabel)}</strong>
            {entry.status ? (
              <span
                className={cn(
                  "inline-flex rounded-full border border-af-border bg-af-surface-raised px-2 py-0.5 text-af-text-subtle",
                  DASHBOARD_SUPPORTING_TEXT_CLASS,
                )}
              >
                {entry.status}
              </span>
            ) : null}
          </div>
          {metadata.length > 0 || timestampState.label ? (
            <div className="grid gap-1">
              <div
                className={cn(
                  "flex flex-wrap items-center gap-x-2 gap-y-1 text-af-text-subtle",
                  DASHBOARD_SUPPORTING_TEXT_CLASS,
                )}
              >
                {metadata.map((item) => (
                  <span key={item}>{item}</span>
                ))}
                {timestampState.label ? (
                  <span title={timestampState.rawTimestamp ?? undefined}>
                    {timestampState.label}
                  </span>
                ) : null}
              </div>
            </div>
          ) : null}
        </div>
      </div>
      <TranscriptEntryBody entry={entry} locale={locale} />
    </article>
  );
}

function getTranscriptTimestampState(
  timestamp: string | undefined,
  locale?: string,
) {
  const messages = getProviderSessionDetailMessages(locale);
  return getLocalDateTimeDisplay(timestamp, messages.unavailableValue, locale, {
    missingLabel: messages.noTimestamp,
  });
}

function TranscriptEntryBody({
  entry,
  locale,
}: {
  entry: TranscriptEntry;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const encryptedContent =
    entry.type === "reasoning" ? entry.encryptedContent?.trim() : undefined;

  switch (entry.type) {
    case "user_message":
    case "assistant_message":
      return entry.text ? <AuthoredBodyText value={entry.text} /> : null;
    case "reasoning":
      return (
        <div className="grid gap-2">
          {entry.encrypted && !entry.text ? (
            <EncryptedReasoningNotice locale={locale} />
          ) : null}
          {entry.summary ? (
            <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>
              {entry.summary}
            </p>
          ) : null}
          {entry.text ? <CodePanel value={entry.text} /> : null}
          {encryptedContent ? (
            <ExpandableCodeBlock
              label={messages.encryptedReasoningStateLabel}
              locale={locale}
              value={encryptedContent}
            />
          ) : null}
          {entry.encrypted && !entry.text && !encryptedContent ? (
            <p
              className={cn(
                "m-0 text-af-text-subtle",
                DASHBOARD_SUPPORTING_TEXT_CLASS,
              )}
            >
              {messages.encryptedReasoningOnly}
            </p>
          ) : null}
        </div>
      );
    case "tool_call":
      return (
        <div className="grid gap-3">
          {entry.text ? (
            <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>{entry.text}</p>
          ) : null}
          {entry.arguments ? (
            <ExpandableCodeBlock
              label={messages.argumentsLabel}
              locale={locale}
              value={entry.arguments}
            />
          ) : null}
        </div>
      );
    case "tool_output":
      if (entry.name === "exec_command" && entry.output) {
        return (
          <FriendlyExecCommandOutput
            locale={locale}
            output={entry.output}
            status={entry.status}
            text={entry.text}
          />
        );
      }

      return (
        <div className="grid gap-3">
          {entry.text ? (
            <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>{entry.text}</p>
          ) : null}
          {entry.output ? (
            <ExpandableCodeBlock
              label={messages.outputLabel}
              locale={locale}
              value={entry.output}
            />
          ) : null}
        </div>
      );
    case "system_event":
      return (
        <div className="grid gap-2">
          {entry.summary ? (
            <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>
              {entry.summary}
            </p>
          ) : null}
          {entry.text ? <CodePanel value={entry.text} /> : null}
        </div>
      );
  }
}

function getTranscriptEntryLabel(
  entry: TranscriptEntry,
  messages: ReturnType<typeof getProviderSessionDetailMessages>,
) {
  switch (entry.type) {
    case "user_message":
      return messages.userMessageLabel;
    case "assistant_message":
      return messages.assistantMessageLabel;
    case "reasoning":
      return messages.reasoningTranscriptLabel;
    case "tool_call":
      return messages.toolCallLabel;
    case "tool_output":
      return messages.toolOutputLabel;
    case "system_event":
      return messages.systemEventLabel;
  }
}

function getTranscriptEntryTitle(entry: TranscriptEntry, defaultLabel: string) {
  switch (entry.type) {
    case "tool_call":
      return entry.name ?? defaultLabel;
    case "tool_output":
      return entry.callId ?? defaultLabel;
    case "system_event":
      return entry.sourceType ?? defaultLabel;
    default:
      return defaultLabel;
  }
}

function getTranscriptEntryClassName(entryType: TranscriptEntry["type"]) {
  return TRANSCRIPT_ENTRY_CLASS_NAMES[entryType];
}
