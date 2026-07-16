import { useId, useState } from "react";
import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import {
  AlertPanel,
  DashboardStatusPill,
  Heading,
  Label,
  Text,
} from "../../../components/ui";
import { ExpandablePanelTrigger } from "../../../components/ui/expandable-panel-trigger";
import { getLocalDateTimeDisplay } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";
import { FriendlyExecCommandOutput } from "./exec-command-output";
import { TranscriptContentPanel } from "./expandable-transcript-content";

type SessionDetail = ProviderSessionDetailResponse;
type TranscriptEntry = SessionDetail["transcript"][number];

const TRANSCRIPT_PREVIEW_CHAR_LIMIT = 220;

export function TranscriptSection({
  className,
  detail,
  locale,
  showHeading = true,
}: {
  className?: string;
  detail: SessionDetail;
  locale?: string;
  showHeading?: boolean;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className={cn("grid gap-3", className)}>
      {showHeading ? (
        <Heading as="h5">{messages.transcriptHeading}</Heading>
      ) : null}
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
    <AlertPanel className={className} radius="lg" tone="info">
      <DashboardStatusPill size="compact" tone="info">
        {messages.encryptedReasoningStateLabel}
      </DashboardStatusPill>
      <Text className="m-0 text-on-surface-variant">
        {messages.encryptedReasoningDescription}
      </Text>
    </AlertPanel>
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
  const entryTitle = getTranscriptEntryTitle(entry, entryLabel);
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
  const [expanded, setExpanded] = useState(true);
  const bodyID = useId();
  const hasBody = hasTranscriptEntryBody(entry);
  const preview = getTranscriptEntryPreview(entry, messages);

  return (
    <article className="grid gap-2 py-1.5">
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <Label>{entryTitle}</Label>
            {entry.status ? (
              <DashboardStatusPill size="compact">
                {entry.status}
              </DashboardStatusPill>
            ) : null}
          </div>
          {metadata.length > 0 || timestampState.label ? (
            <Text
              as="div"
              className="flex flex-wrap items-center gap-x-2 gap-y-1 text-on-surface-subtle"
              variant="supporting"
            >
              {metadata.map((item) => (
                <span key={item}>{item}</span>
              ))}
              {timestampState.label ? (
                <span title={timestampState.rawTimestamp ?? undefined}>
                  {timestampState.label}
                </span>
              ) : null}
            </Text>
          ) : null}
          {hasBody && !expanded && preview ? (
            <Text className="m-0 line-clamp-2">{preview}</Text>
          ) : null}
        </div>
        {hasBody ? (
          <ExpandablePanelTrigger
            aria-label={messages.transcriptToggleLabel({
              expanded,
              section: entryTitle,
            })}
            className="mt-0.5 h-10 min-h-0 w-10 rounded-lg"
            controlsID={bodyID}
            expanded={expanded}
            onClick={() => setExpanded((current) => !current)}
            variant="outline"
          />
        ) : null}
      </div>
      {hasBody && expanded ? (
        <div id={bodyID}>
          <TranscriptEntryBody entry={entry} locale={locale} />
        </div>
      ) : null}
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
      return entry.text ? <TranscriptContentPanel value={entry.text} /> : null;
    case "reasoning":
      return (
        <div className="grid gap-2">
          {entry.encrypted && !entry.text ? (
            <EncryptedReasoningNotice locale={locale} />
          ) : null}
          {entry.summary ? (
            <TranscriptContentSection
              label={messages.execCommandOutputSummaryLabel}
              value={entry.summary}
            />
          ) : null}
          {entry.text ? (
            <TranscriptContentSection
              kind="code"
              label={messages.reasoningTranscriptLabel}
              value={entry.text}
            />
          ) : null}
          {encryptedContent ? (
            <TranscriptContentSection
              kind="code"
              label={messages.encryptedReasoningStateLabel}
              value={encryptedContent}
            />
          ) : null}
          {entry.encrypted && !entry.text && !encryptedContent ? (
            <Text className="m-0 text-on-surface-subtle" variant="supporting">
              {messages.encryptedReasoningOnly}
            </Text>
          ) : null}
        </div>
      );
    case "tool_call":
      return (
        <div className="grid gap-3">
          {entry.text ? (
            <TranscriptContentSection
              label={messages.toolCallLabel}
              value={entry.text}
            />
          ) : null}
          {entry.arguments ? (
            <TranscriptContentSection
              kind="code"
              label={messages.argumentsLabel}
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
            <TranscriptContentSection
              label={messages.toolOutputLabel}
              value={entry.text}
            />
          ) : null}
          {entry.output ? (
            <TranscriptContentSection
              kind="code"
              label={messages.outputLabel}
              value={entry.output}
            />
          ) : null}
        </div>
      );
    case "system_event":
      return (
        <div className="grid gap-2">
          {entry.summary ? (
            <TranscriptContentSection
              label={messages.execCommandOutputSummaryLabel}
              value={entry.summary}
            />
          ) : null}
          {entry.text ? (
            <TranscriptContentSection
              kind="code"
              label={messages.systemEventLabel}
              value={entry.text}
            />
          ) : null}
        </div>
      );
  }
}

function TranscriptContentSection({
  kind = "text",
  label,
  value,
}: {
  kind?: "code" | "text";
  label: string;
  value: string;
}) {
  return (
    <section className="grid gap-2">
      <Label>{label}</Label>
      <TranscriptContentPanel kind={kind} value={value} />
    </section>
  );
}

function hasTranscriptEntryBody(entry: TranscriptEntry) {
  switch (entry.type) {
    case "user_message":
    case "assistant_message":
      return Boolean(entry.text);
    case "reasoning":
      return Boolean(
        entry.text ||
          entry.summary ||
          entry.encryptedContent ||
          (entry.encrypted && !entry.text),
      );
    case "tool_call":
      return Boolean(entry.text || entry.arguments);
    case "tool_output":
      return Boolean(entry.text || entry.output);
    case "system_event":
      return Boolean(entry.text || entry.summary);
  }
}

function getTranscriptEntryPreview(
  entry: TranscriptEntry,
  messages: ReturnType<typeof getProviderSessionDetailMessages>,
) {
  let value: string | undefined;

  switch (entry.type) {
    case "user_message":
    case "assistant_message":
      value = entry.text;
      break;
    case "reasoning":
      value =
        entry.summary ??
        entry.text ??
        entry.encryptedContent ??
        (entry.encrypted ? messages.encryptedReasoningOnly : undefined);
      break;
    case "tool_call":
      value = entry.text ?? entry.arguments;
      break;
    case "tool_output":
      value = entry.text ?? entry.output;
      break;
    case "system_event":
      value = entry.summary ?? entry.text;
      break;
  }

  return getTranscriptPreviewText(value);
}

function getTranscriptPreviewText(value: string | undefined) {
  const preview = value?.replace(/\s+/g, " ").trim();

  if (!preview) {
    return null;
  }

  if (preview.length <= TRANSCRIPT_PREVIEW_CHAR_LIMIT) {
    return preview;
  }

  return `${preview.slice(0, TRANSCRIPT_PREVIEW_CHAR_LIMIT).trimEnd()}...`;
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
