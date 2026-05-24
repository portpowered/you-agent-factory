import { useId, useState } from "react";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import { cn } from "../../../lib/cn";
import {
  AuthoredBodyText,
  PROVIDER_SESSION_CARD_CLASS,
  PROVIDER_SESSION_CODE_PANEL_CLASS,
  PROVIDER_SESSION_STATUS_PILL_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
} from "./detail-card-shared";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";

type SessionDetail = ProviderSessionDetailResponse;
type TranscriptEntry = SessionDetail["transcript"][number];

const TRANSCRIPT_COLLAPSE_CHAR_LIMIT = 320;
const TRANSCRIPT_ENTRY_CLASS_NAMES: Record<TranscriptEntry["type"], string> = {
  assistant_message: "border-af-border bg-af-surface-subtle",
  reasoning: "border-af-info-border bg-af-info-surface",
  system_event: "border-af-border bg-af-surface-raised",
  tool_call: "border-af-warning-border bg-af-warning-surface",
  tool_output: "border-af-success-border bg-af-success-surface",
  user_message: "border-af-accent-border bg-af-accent-surface",
};
const TRANSCRIPT_BADGE_CLASS_NAMES: Record<TranscriptEntry["type"], string> = {
  assistant_message: "border-af-border bg-af-surface-raised text-af-text-muted",
  reasoning: "border-af-info-border bg-af-info-surface text-af-info-text",
  system_event: "border-af-border bg-af-surface-raised text-af-text-subtle",
  tool_call: "border-af-warning-border bg-af-warning-surface text-af-warning-text",
  tool_output: "border-af-success-border bg-af-success-surface text-af-success-text",
  user_message: "border-af-accent-border bg-af-accent-surface text-af-text",
};

export function TranscriptSection({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className="grid gap-2.5">
      <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>{messages.transcriptHeading}</h5>
      <div className="grid gap-3">
        {detail.transcript.map((entry) => (
          <TranscriptEntryCard entry={entry} key={entry.order} locale={locale} />
        ))}
      </div>
    </section>
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
    entry.timestamp
      ? messages.transcriptTimestampLabel({ timestamp: entry.timestamp })
      : null,
    entry.lineNumber
      ? messages.transcriptLineNumberLabel({ lineNumber: entry.lineNumber })
      : null,
  ].filter(Boolean);

  return (
    <article
      className={cn(
        PROVIDER_SESSION_CARD_CLASS,
        "grid gap-3",
        getTranscriptEntryClassName(entry.type),
      )}
    >
      <div className="grid gap-2 md:grid-cols-[auto_minmax(0,1fr)] md:items-start md:gap-3">
        <span
          className={cn(
            "inline-flex w-fit rounded-full border px-2 py-0.5",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
            getTranscriptBadgeClassName(entry.type),
          )}
        >
          {entryLabel}
        </span>
        <div className="grid gap-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <strong>{getTranscriptEntryTitle(entry, entryLabel)}</strong>
            {entry.status ? (
              <span
                className={cn(
                  PROVIDER_SESSION_STATUS_PILL_CLASS,
                )}
              >
                {entry.status}
              </span>
            ) : null}
          </div>
          {metadata.length > 0 ? (
            <p className={REQUEST_SELECTION_STATUS_CLASS}>
              {metadata.join(messages.transcriptMetadataSeparator)}
            </p>
          ) : null}
        </div>
      </div>
      <TranscriptEntryBody entry={entry} locale={locale} />
    </article>
  );
}

function TranscriptEntryBody({
  entry,
  locale,
}: {
  entry: TranscriptEntry;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  switch (entry.type) {
    case "user_message":
    case "assistant_message":
      return entry.text ? <AuthoredBodyText value={entry.text} /> : null;
    case "reasoning":
      return (
        <div className="grid gap-2">
          {entry.summary ? (
            <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>{entry.summary}</p>
          ) : null}
          {entry.text ? <CodePanel value={entry.text} /> : null}
          {entry.encrypted && !entry.text ? (
            <p className={REQUEST_SELECTION_STATUS_CLASS}>{messages.encryptedReasoningOnly}</p>
          ) : null}
        </div>
      );
    case "tool_call":
      return (
        <div className="grid gap-3">
          {entry.text ? <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>{entry.text}</p> : null}
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
      return (
        <div className="grid gap-3">
          {entry.text ? <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>{entry.text}</p> : null}
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
            <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>{entry.summary}</p>
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

function getTranscriptBadgeClassName(entryType: TranscriptEntry["type"]) {
  return TRANSCRIPT_BADGE_CLASS_NAMES[entryType];
}

function ExpandableCodeBlock({
  label,
  locale,
  value,
}: {
  label: string;
  locale?: string;
  value: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const [expanded, setExpanded] = useState(false);
  const panelID = useId();
  const shouldCollapse = value.length > TRANSCRIPT_COLLAPSE_CHAR_LIMIT;

  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
        {shouldCollapse ? (
          <button
            aria-controls={panelID}
            aria-expanded={expanded}
            className={cn(
              "inline-flex w-fit rounded-lg border border-af-border bg-af-surface-raised px-2.5 py-2 text-af-text-muted transition hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
            onClick={() => setExpanded((current) => !current)}
            type="button"
          >
            {messages.transcriptToggleLabel({ expanded, section: label })}
          </button>
        ) : null}
      </div>
      <div id={panelID}>
        <CodePanel
          value={
            shouldCollapse && !expanded
              ? `${value.slice(0, TRANSCRIPT_COLLAPSE_CHAR_LIMIT)}…`
              : value
          }
        />
      </div>
    </div>
  );
}

function CodePanel({ value }: { value: string }) {
  return (
    <pre
      className={PROVIDER_SESSION_CODE_PANEL_CLASS}
    >
      {value}
    </pre>
  );
}
