import { Button, Code } from "@you-agent-factory/components/primitives";
import type {
  TraceRelationPathEndpoint,
  TraceRelationPathEntry,
} from "../lib/trace-relation-path";
import type { TraceSelectionIdentity } from "../lib/trace-selection";
import {
  traceSelectionKey,
  traceSelectionMatches,
} from "../lib/trace-selection";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

export interface TraceRelationPathProps {
  entries: readonly TraceRelationPathEntry[];
  locale?: string;
  onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedTraceSelection?: TraceSelectionIdentity | null;
}

export function TraceRelationPath({
  entries,
  locale,
  onSelectTraceSelection,
  onSelectWorkID,
  selectedTraceSelection,
}: TraceRelationPathProps) {
  const messages = getTraceDrilldownMessages(locale);

  return (
    <section
      aria-label={messages.relationPathLabel}
      className="grid min-w-0 gap-2"
      data-trace-relation-path
    >
      {entries.length === 0 ? (
        <p
          className="m-0 text-sm text-on-surface-variant"
          data-trace-relation-path-empty
          role="status"
        >
          {messages.relationPathEmpty}
        </p>
      ) : (
        <ol className="m-0 grid min-w-0 gap-2 p-0">
          {entries.map((entry) => (
            <TraceRelationPathEntryView
              entry={entry}
              key={entry.id}
              locale={locale}
              messages={messages}
              onSelectTraceSelection={onSelectTraceSelection}
              onSelectWorkID={onSelectWorkID}
              selectedTraceSelection={selectedTraceSelection}
            />
          ))}
        </ol>
      )}
    </section>
  );
}

function TraceRelationPathEntryView({
  entry,
  locale,
  messages,
  onSelectTraceSelection,
  onSelectWorkID,
  selectedTraceSelection,
}: Omit<TraceRelationPathProps, "entries"> & {
  entry: TraceRelationPathEntry;
  messages: ReturnType<typeof getTraceDrilldownMessages>;
}) {
  const relationLabel =
    entry.kind === "predecessor"
      ? messages.predecessorRelationLabel
      : messages.localizeRelationType(entry.relationType);
  const sourceLabel =
    entry.kind === "predecessor"
      ? messages.predecessorSourceLabel
      : messages.relationPathSourceLabel;
  const targetLabel =
    entry.kind === "predecessor"
      ? messages.predecessorTargetLabel
      : messages.relationPathTargetLabel;

  return (
    <li
      className="grid min-w-0 gap-2 rounded-lg border border-outline bg-surface-container-high p-3"
      data-trace-relation-entry
      data-trace-relation-id={entry.id}
    >
      <div className="flex min-w-0 flex-wrap items-baseline gap-2">
        <span className="font-semibold text-on-surface">{relationLabel}</span>
        {entry.requiredState ? (
          <span className="text-sm text-on-surface-variant">
            {messages.relationPathRequiredStateLabel}:{" "}
            {messages.localizeRelationState(entry.requiredState)}
          </span>
        ) : null}
        {entry.requestID ? (
          <span className="text-sm text-on-surface-variant">
            {messages.relationPathRequestIDLabel}:{" "}
            <Code size="supporting">{entry.requestID}</Code>
          </span>
        ) : null}
      </div>

      <div className="grid min-w-0 gap-2 sm:flex sm:items-start sm:gap-3">
        <TraceRelationPathEndpointView
          endpoint={entry.source}
          endpointLabel={sourceLabel}
          locale={locale}
          messages={messages}
          onSelectTraceSelection={onSelectTraceSelection}
          onSelectWorkID={onSelectWorkID}
          selectedTraceSelection={selectedTraceSelection}
        />
        <span
          aria-hidden="true"
          className="hidden shrink-0 self-center text-on-surface-variant sm:block"
        >
          →
        </span>
        <TraceRelationPathEndpointView
          endpoint={entry.target}
          endpointLabel={targetLabel}
          locale={locale}
          messages={messages}
          onSelectTraceSelection={onSelectTraceSelection}
          onSelectWorkID={onSelectWorkID}
          selectedTraceSelection={selectedTraceSelection}
        />
      </div>
    </li>
  );
}

function TraceRelationPathEndpointView({
  endpoint,
  endpointLabel,
  messages,
  onSelectTraceSelection,
  onSelectWorkID,
  selectedTraceSelection,
}: Omit<TraceRelationPathProps, "entries"> & {
  endpoint: TraceRelationPathEndpoint;
  endpointLabel: string;
  messages: ReturnType<typeof getTraceDrilldownMessages>;
}) {
  return (
    <div className="grid min-w-0 flex-1 gap-1">
      <span className="text-xs font-semibold uppercase tracking-wide text-on-surface-variant">
        {endpointLabel}
      </span>
      <span className="break-words font-semibold text-on-surface">
        {endpoint.label}
      </span>
      {endpoint.dispatchID ? (
        <Code className="max-w-full break-all" size="supporting">
          {endpoint.dispatchID}
        </Code>
      ) : null}
      {endpoint.selectionIdentities.length > 0 ? (
        <ul className="m-0 grid min-w-0 gap-1 p-0">
          {endpoint.selectionIdentities.map((selection) => (
            <li className="list-none" key={traceSelectionKey(selection)}>
              {onSelectTraceSelection || onSelectWorkID ? (
                <Button
                  aria-pressed={traceSelectionMatches(
                    selection,
                    selectedTraceSelection,
                  )}
                  className="h-auto min-h-11 max-w-full justify-start whitespace-normal px-2.5 py-2 text-left"
                  data-trace-selection-key={traceSelectionKey(selection)}
                  data-trace-selection-surface="text"
                  onClick={() => {
                    onSelectTraceSelection?.(selection);
                    if (!onSelectTraceSelection && selection.work_id) {
                      onSelectWorkID?.(selection.work_id);
                    }
                  }}
                  title={messages.traceSelectionIdentityLabel(
                    selection.dispatch_id,
                    selection.work_id,
                    selection.attempt,
                  )}
                  tone="outline"
                >
                  {messages.traceSelectionIdentityLabel(
                    selection.dispatch_id,
                    selection.work_id,
                    selection.attempt,
                  )}
                </Button>
              ) : (
                <Code size="supporting">
                  {messages.traceSelectionIdentityLabel(
                    selection.dispatch_id,
                    selection.work_id,
                    selection.attempt,
                  )}
                </Code>
              )}
            </li>
          ))}
        </ul>
      ) : endpoint.workID ? (
        onSelectWorkID ? (
          <Button
            className="h-auto min-h-11 max-w-full justify-start whitespace-normal px-2.5 py-2 text-left"
            onClick={() => onSelectWorkID(endpoint.workID ?? "")}
            title={endpoint.workID}
            tone="outline"
          >
            {messages.selectWorkLabel(endpoint.workID)}
          </Button>
        ) : (
          <Code size="supporting">{endpoint.workID}</Code>
        )
      ) : null}
    </div>
  );
}
