import type { ReactNode } from "react";
import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import { Label, Text } from "@you-agent-factory/components/primitives";
import { StandardExpandableSection } from "../../standard-card-components/components/standard-expandable-section";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";

type SessionDetail = ProviderSessionDetailResponse;
const TRANSCRIPT_SECTION_PREVIEW_LIMIT = 3;

export function ProviderSessionExpandableSection({
  children,
  defaultExpanded,
  heading,
  locale,
  preview,
  resetKey,
}: {
  children: ReactNode;
  defaultExpanded?: boolean;
  heading: string;
  locale?: string;
  preview: ReactNode;
  resetKey?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);

  return (
    <StandardExpandableSection
      defaultExpanded={defaultExpanded}
      heading={heading}
      preview={preview}
      resetKey={resetKey}
      toggleLabel={({ expanded, section }) =>
        messages.transcriptToggleLabel({ expanded, section })
      }
    >
      {children}
    </StandardExpandableSection>
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
          <Label as="dt">{item.label}</Label>
          <Text as="dd" className="m-0 [overflow-wrap:anywhere]">
            {item.value}
          </Text>
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
          <Label>{getTranscriptPreviewEntryTitle(entry, messages)}</Label>
          <Text className="m-0">
            {messages.orderLabel({
              order: entry.order,
              turnIndex: entry.turnIndex,
            })}
          </Text>
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
