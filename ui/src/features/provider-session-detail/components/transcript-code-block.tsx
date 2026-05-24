import { useId, useState } from "react";

import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";

const TRANSCRIPT_COLLAPSE_CHAR_LIMIT = 320;

export function ExpandableCodeBlock({
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
              "inline-flex w-fit rounded-lg border border-af-overlay/12 bg-af-overlay/6 px-2.5 py-2 text-af-ink/78 transition hover:border-af-overlay/18 hover:bg-af-overlay/10 hover:text-af-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent",
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

export function CodePanel({ value }: { value: string }) {
  return (
    <pre
      className={cn(
        "m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-3 [overflow-wrap:anywhere]",
        DASHBOARD_BODY_CODE_CLASS,
      )}
    >
      {value}
    </pre>
  );
}
