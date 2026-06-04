import { useId, useState } from "react";

import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
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
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
        {shouldCollapse ? (
          <button
            aria-controls={panelID}
            aria-expanded={expanded}
            className={cn(
              "inline-flex w-fit rounded-lg border border-outline px-2.5 py-2 text-on-surface-variant transition hover:border-outline-variant hover:bg-af-overlay hover:text-on-surface focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
            onClick={() => setExpanded((current) => !current)}
            type="button"
          >
            {messages.transcriptToggleLabel({ expanded, section: label })}
          </button>
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
      <pre
        className={cn(
          "m-0 whitespace-pre-wrap rounded-lg border border-outline bg-surface-container-low p-3 [overflow-wrap:anywhere]",
          DASHBOARD_BODY_CODE_CLASS,
        )}
        id={id}
      >
        {value}
      </pre>
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
