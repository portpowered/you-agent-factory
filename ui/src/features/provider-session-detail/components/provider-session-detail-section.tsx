import { type ReactNode, useId, useState } from "react";
import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import { ExpandablePanelTrigger } from "../../../components/ui/expandable-panel-trigger";
import { cn } from "../../../lib/cn";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";

type SessionDetail = ProviderSessionDetailResponse;
const TRANSCRIPT_SECTION_PREVIEW_LIMIT = 3;

export function ProviderSessionExpandableSection({
  children,
  heading,
  locale,
  preview,
}: {
  children: ReactNode;
  heading: string;
  locale?: string;
  preview: ReactNode;
}) {
  const [expanded, setExpanded] = useState(false);
  const contentID = useId();
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <section className="grid gap-3 py-1.5">
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0">
          <h5 className={DASHBOARD_SECTION_HEADING_CLASS}>{heading}</h5>
        </div>
        <ExpandablePanelTrigger
          aria-label={messages.transcriptToggleLabel({
            expanded,
            section: heading,
          })}
          className="mt-0.5 h-10 min-h-0 w-10 rounded-lg"
          controlsID={contentID}
          expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          variant="outline"
        />
      </div>
      <div className="grid gap-5" id={contentID}>
        {expanded ? children : preview}
      </div>
    </section>
  );
}

export function SectionMetricPreview({
  items,
}: {
  items: { label: string; value: number | string | ReactNode }[];
}) {
  return (
    <dl className="grid gap-2">
      {items.map((item) => (
        <div className="grid gap-1" key={item.label}>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{item.label}</dt>
          <dd
            className={cn(
              "m-0 [overflow-wrap:anywhere]",
              DASHBOARD_BODY_TEXT_CLASS,
            )}
          >
            {item.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

export function TranscriptSectionPreview({
  detail,
  locale,
}: {
  detail: SessionDetail;
  locale?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const previewEntries = detail.transcript.slice(
    0,
    TRANSCRIPT_SECTION_PREVIEW_LIMIT,
  );

  return (
    <div className="grid gap-2">
      {previewEntries.map((entry) => (
        <div className="grid gap-1" key={entry.order}>
          <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {getTranscriptPreviewEntryTitle(entry, messages)}
          </span>
          <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS)}>
            {messages.orderLabel({
              order: entry.order,
              turnIndex: entry.turnIndex,
            })}
          </p>
        </div>
      ))}
    </div>
  );
}

function getTranscriptPreviewEntryTitle(
  entry: SessionDetail["transcript"][number],
  messages: ReturnType<typeof getProviderSessionDetailMessages>,
) {
  switch (entry.type) {
    case "tool_call":
      return entry.name ?? messages.toolCallLabel;
    case "tool_output":
      return entry.callId ?? messages.toolOutputLabel;
    case "system_event":
      return entry.sourceType ?? messages.systemEventLabel;
    case "assistant_message":
      return messages.assistantMessageLabel;
    case "reasoning":
      return messages.reasoningTranscriptLabel;
    case "user_message":
      return messages.userMessageLabel;
  }
}
