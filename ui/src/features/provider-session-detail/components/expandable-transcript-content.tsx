import { useId, useState } from "react";

import { Button, CodePanel, DashboardLabel } from "../../../components/ui";
import { AuthoredBodyText } from "../../../lib/authored-body-text";
import { cn } from "../../../lib/cn";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";

const TRANSCRIPT_COLLAPSE_CHAR_LIMIT = 320;

export function ExpandableTranscriptContent({
  className,
  kind = "text",
  label,
  locale,
  value,
}: {
  className?: string;
  kind?: "code" | "text";
  label: string;
  locale?: string;
  value: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const [expanded, setExpanded] = useState(false);
  const panelID = useId();
  const shouldCollapse = value.length > TRANSCRIPT_COLLAPSE_CHAR_LIMIT;
  const displayValue =
    shouldCollapse && !expanded
      ? `${value.slice(0, TRANSCRIPT_COLLAPSE_CHAR_LIMIT)}...`
      : value;

  return (
    <section className={cn("grid gap-2", className)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <DashboardLabel>{label}</DashboardLabel>
        {shouldCollapse ? (
          <Button
            aria-controls={panelID}
            aria-expanded={expanded}
            className="w-fit rounded-lg"
            onClick={() => setExpanded((current) => !current)}
            size="sm"
            tone="outline"
            type="button"
          >
            {messages.transcriptToggleLabel({ expanded, section: label })}
          </Button>
        ) : null}
      </div>
      <TranscriptContentPanel
        id={panelID}
        kind={kind}
        value={displayValue}
      />
    </section>
  );
}

export function TranscriptContentPanel({
  id,
  kind = "text",
  value,
}: {
  id?: string;
  kind?: "code" | "text";
  value: string;
}) {
  if (kind === "code") {
    return (
      <CodePanel id={id} padding="default" surface="low">
        {value}
      </CodePanel>
    );
  }

  return (
    <div id={id}>
      <AuthoredBodyText
        className="border-0 bg-transparent p-0"
        value={value}
      />
    </div>
  );
}
