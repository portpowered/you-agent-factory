// biome-ignore lint/style/noExcessiveLinesPerFile: structured timeline detail renderers stay colocated so every provider-neutral fact shares one disclosure contract.
import type {
  WorkerSessionTimelineEntry,
  WorkerTimelineContentBlock,
} from "../lib/worker-session-timeline-projection-types";
import type { WorkerSessionTimelineMessages } from "../messages/worker-session-timeline";
import { WorkerSessionTimelineContentBlockDetails } from "./worker-session-timeline-content-block-details";
import {
  BoundedCode,
  BoundedText,
  DetailList,
  DetailSection,
  DetailValue,
  NavigableDetail,
} from "./worker-session-timeline-detail-primitives";

export interface WorkerSessionTimelineEntryDetailsProps {
  entry: WorkerSessionTimelineEntry;
  messages: WorkerSessionTimelineMessages;
  onNavigateToWorkerSession?: (workerSessionID: string) => void;
  position?: number;
  totalEntries?: number;
}

export function EntryStructuredDetails({
  entry,
  messages,
  onNavigateToWorkerSession,
}: WorkerSessionTimelineEntryDetailsProps) {
  return (
    <>
      {entry.attempt ? (
        <AttemptDetails attempt={entry.attempt} messages={messages} />
      ) : null}
      {entry.continuation ? (
        <ContinuationDetails
          continuation={entry.continuation}
          messages={messages}
          onNavigateToWorkerSession={onNavigateToWorkerSession}
        />
      ) : null}
      {entry.message ? (
        <MessageDetails message={entry.message} messages={messages} />
      ) : null}
      {entry.reasoning ? (
        <ReasoningDetails messages={messages} reasoning={entry.reasoning} />
      ) : null}
      {entry.progress ? (
        <ProgressDetails messages={messages} progress={entry.progress} />
      ) : null}
      {entry.tool ? (
        <ToolDetails messages={messages} tool={entry.tool} />
      ) : null}
      {entry.usage ? (
        <UsageDetails messages={messages} usage={entry.usage} />
      ) : null}
      {entry.failure ? (
        <FailureDetails failure={entry.failure} messages={messages} />
      ) : null}
      {entry.terminal ? (
        <TerminalDetails messages={messages} terminal={entry.terminal} />
      ) : null}
      {entry.generic ? (
        <GenericDetails generic={entry.generic} messages={messages} />
      ) : null}
      <CanonicalSourceDetails entry={entry} messages={messages} />
    </>
  );
}

function AttemptDetails({
  attempt,
  messages,
}: {
  attempt: NonNullable<WorkerSessionTimelineEntry["attempt"]>;
  messages: WorkerSessionTimelineMessages;
}) {
  return (
    <DetailSection heading={messages.attemptLabel(attempt.number, attempt.id)}>
      <DetailList
        items={[
          attempt.id
            ? { label: messages.attemptIDLabel, value: attempt.id }
            : null,
          attempt.reason
            ? { label: messages.retryReasonLabel, value: attempt.reason }
            : null,
          attempt.dispatchId
            ? { label: messages.dispatchLabel, value: attempt.dispatchId }
            : null,
          attempt.turnId
            ? { label: messages.turnLabel, value: attempt.turnId }
            : null,
          attempt.status
            ? { label: messages.toolStatusLabel, value: attempt.status }
            : null,
        ]}
      />
    </DetailSection>
  );
}

function ContinuationDetails({
  continuation,
  messages,
  onNavigateToWorkerSession,
}: {
  continuation: NonNullable<WorkerSessionTimelineEntry["continuation"]>;
  messages: WorkerSessionTimelineMessages;
  onNavigateToWorkerSession?: (workerSessionID: string) => void;
}) {
  return (
    <DetailSection heading={messages.continuationLabel}>
      <div className="grid min-w-0 gap-2">
        {continuation.predecessorWorkerSessionId ? (
          <NavigableDetail
            actionLabel={messages.openWorkerSessionLabel(
              continuation.predecessorWorkerSessionId,
            )}
            label={messages.predecessorLabel}
            onNavigate={onNavigateToWorkerSession}
            value={continuation.predecessorWorkerSessionId}
          />
        ) : null}
        {continuation.successorWorkerSessionId ? (
          <NavigableDetail
            actionLabel={messages.openWorkerSessionLabel(
              continuation.successorWorkerSessionId,
            )}
            label={messages.successorLabel}
            onNavigate={onNavigateToWorkerSession}
            value={continuation.successorWorkerSessionId}
          />
        ) : null}
        {continuation.previousDispatchId ? (
          <DetailValue
            label={messages.dispatchLabel}
            value={continuation.previousDispatchId}
          />
        ) : null}
        {continuation.previousAttemptId ? (
          <DetailValue
            label={messages.attemptIDLabel}
            value={continuation.previousAttemptId}
          />
        ) : null}
        {continuation.providerSession ? (
          <DetailValue
            label={messages.providerSessionLabel}
            value={`${continuation.providerSession.provider} / ${continuation.providerSession.kind} / ${continuation.providerSession.id}`}
          />
        ) : null}
      </div>
    </DetailSection>
  );
}

function MessageDetails({
  message,
  messages,
}: {
  message: NonNullable<WorkerSessionTimelineEntry["message"]>;
  messages: WorkerSessionTimelineMessages;
}) {
  const messageText = message.text ?? message.delta?.textDelta;
  return (
    <DetailSection heading={messages.messageTextLabel}>
      <div className="grid min-w-0 gap-3">
        {message.role ? (
          <DetailValue
            label={messages.messageRoleLabel(message.role)}
            value={message.role}
          />
        ) : null}
        {messageText ? (
          <BoundedText
            collapseLabel={messages.collapseContentAction}
            expandLabel={messages.expandContentAction}
            label={messages.messageTextLabel}
            value={messageText}
          />
        ) : null}
        {message.delta ? (
          <DetailList
            items={[
              message.delta.contentBlockIndex === undefined
                ? null
                : {
                    label: messages.sourceSequenceLabel(
                      message.delta.contentBlockIndex,
                    ),
                    value: String(message.delta.contentBlockIndex),
                  },
              message.delta.contentBlockKind
                ? {
                    label: messages.sourceLabel,
                    value: message.delta.contentBlockKind,
                  }
                : null,
              message.partial === undefined
                ? null
                : {
                    label: messages.phaseLabel("PARTIAL"),
                    value: String(message.partial),
                  },
            ]}
          />
        ) : null}
        {message.contentBlocks?.map((block) => (
          <WorkerSessionTimelineContentBlockDetails
            block={block}
            key={contentBlockKey(block)}
            messages={messages}
          />
        ))}
      </div>
    </DetailSection>
  );
}

function ReasoningDetails({
  messages,
  reasoning,
}: {
  messages: WorkerSessionTimelineMessages;
  reasoning: NonNullable<WorkerSessionTimelineEntry["reasoning"]>;
}) {
  return (
    <DetailSection heading={messages.reasoningLabel}>
      <DetailList
        items={[
          reasoning.representation === "SNAPSHOT"
            ? {
                label: messages.reasoningSnapshotLabel,
                value: messages.reasoningSnapshotLabel,
              }
            : reasoning.representation === "DELTA"
              ? {
                  label: messages.reasoningDeltaLabel,
                  value: messages.reasoningDeltaLabel,
                }
              : null,
        ]}
      />
      {reasoning.summary ? (
        <BoundedText
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.reasoningSnapshotLabel}
          value={reasoning.summary}
        />
      ) : null}
      {reasoning.summaryDelta ? (
        <BoundedText
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.reasoningDeltaLabel}
          value={reasoning.summaryDelta}
        />
      ) : null}
    </DetailSection>
  );
}

function ToolDetails({
  messages,
  tool,
}: {
  messages: WorkerSessionTimelineMessages;
  tool: NonNullable<WorkerSessionTimelineEntry["tool"]>;
}) {
  return (
    <DetailSection heading={messages.toolLabel}>
      <DetailList
        items={[
          tool.toolCallId
            ? { label: messages.toolCallIDLabel, value: tool.toolCallId }
            : null,
          tool.toolName
            ? { label: messages.toolNameLabel, value: tool.toolName }
            : null,
          tool.status
            ? { label: messages.toolStatusLabel, value: tool.status }
            : null,
        ]}
      />
      {tool.argumentsSummary !== undefined ? (
        <BoundedCode
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.toolArgumentsLabel}
          value={tool.argumentsSummary}
        />
      ) : null}
      {tool.outputDelta ? (
        <BoundedText
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.toolOutputLabel}
          value={tool.outputDelta}
        />
      ) : null}
      {tool.resultSummary !== undefined ? (
        <BoundedCode
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.toolResultLabel}
          value={tool.resultSummary}
        />
      ) : null}
    </DetailSection>
  );
}

function ProgressDetails({
  messages,
  progress,
}: {
  messages: WorkerSessionTimelineMessages;
  progress: NonNullable<WorkerSessionTimelineEntry["progress"]>;
}) {
  return (
    <DetailSection heading={messages.progressLabel}>
      <DetailList
        items={[
          progress.label
            ? { label: messages.progressLabel, value: progress.label }
            : null,
          progress.percentComplete === undefined
            ? null
            : {
                label: messages.progressPercentLabel,
                value: String(progress.percentComplete),
              },
        ]}
      />
      {progress.message ? (
        <BoundedText
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.progressMessageLabel}
          value={progress.message}
        />
      ) : null}
    </DetailSection>
  );
}

function UsageDetails({
  messages,
  usage,
}: {
  messages: WorkerSessionTimelineMessages;
  usage: NonNullable<WorkerSessionTimelineEntry["usage"]>;
}) {
  return (
    <DetailSection heading={messages.usageLabel}>
      <DetailList
        items={[
          usage.inputTokens === undefined
            ? null
            : {
                label: messages.inputTokensLabel,
                value: String(usage.inputTokens),
              },
          usage.cachedInputTokens === undefined
            ? null
            : {
                label: messages.cachedInputTokensLabel,
                value: String(usage.cachedInputTokens),
              },
          usage.cacheWriteTokens === undefined
            ? null
            : {
                label: messages.cacheWriteTokensLabel,
                value: String(usage.cacheWriteTokens),
              },
          usage.outputTokens === undefined
            ? null
            : {
                label: messages.outputTokensLabel,
                value: String(usage.outputTokens),
              },
          usage.reasoningOutputTokens === undefined
            ? null
            : {
                label: messages.reasoningOutputTokensLabel,
                value: String(usage.reasoningOutputTokens),
              },
          usage.totalTokens === undefined
            ? null
            : {
                label: messages.totalTokensLabel,
                value: String(usage.totalTokens),
              },
          usage.model
            ? { label: messages.usageModelLabel, value: usage.model }
            : null,
        ]}
      />
    </DetailSection>
  );
}

function FailureDetails({
  failure,
  messages,
}: {
  failure: NonNullable<WorkerSessionTimelineEntry["failure"]>;
  messages: WorkerSessionTimelineMessages;
}) {
  return (
    <DetailSection heading={messages.failureLabel}>
      <DetailList
        items={[
          failure.kind
            ? { label: messages.categoryLabel.error, value: failure.kind }
            : null,
          failure.code
            ? { label: messages.sourceLabel, value: failure.code }
            : null,
          failure.retryable === undefined
            ? null
            : {
                label: messages.retryableLabel,
                value: String(failure.retryable),
              },
          failure.retryAfterSeconds === undefined
            ? null
            : {
                label: messages.retryAfterSecondsLabel,
                value: String(failure.retryAfterSeconds),
              },
          failure.retryAttempt === undefined
            ? null
            : {
                label: messages.attemptLabel(failure.retryAttempt, undefined),
                value: String(failure.retryAttempt),
              },
        ]}
      />
      {failure.message ? (
        <BoundedText
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.failureLabel}
          value={failure.message}
        />
      ) : null}
    </DetailSection>
  );
}

function TerminalDetails({
  messages,
  terminal,
}: {
  messages: WorkerSessionTimelineMessages;
  terminal: NonNullable<WorkerSessionTimelineEntry["terminal"]>;
}) {
  return (
    <DetailSection heading={messages.terminalOutcomeHeading}>
      <DetailValue
        label={messages.terminalOutcomeHeading}
        value={messages.terminalOutcomeLabel(terminal.outcome)}
      />
      {terminal.status ? (
        <DetailValue label={messages.toolStatusLabel} value={terminal.status} />
      ) : null}
      {terminal.failure?.message ? (
        <BoundedText
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.failureLabel}
          value={terminal.failure.message}
        />
      ) : null}
    </DetailSection>
  );
}

function GenericDetails({
  generic,
  messages,
}: {
  generic: NonNullable<WorkerSessionTimelineEntry["generic"]>;
  messages: WorkerSessionTimelineMessages;
}) {
  return (
    <DetailSection heading={messages.genericMetadataLabel}>
      <DetailList
        items={[
          {
            label: messages.genericMetadataLabel,
            value: messages.genericSchemaLabel(generic.schemaId),
          },
          {
            label: messages.sourceLabel,
            value: `${generic.sourceType} / ${generic.sourceId}`,
          },
          {
            label: messages.sourceSequenceLabel(generic.sourceSequence),
            value: String(generic.sourceSequence),
          },
          {
            label: messages.messageTextLabel,
            value:
              generic.payloadKeys.length > 0
                ? generic.payloadKeys.join(", ")
                : messages.unknownValueLabel,
          },
        ]}
      />
    </DetailSection>
  );
}

function CanonicalSourceDetails({
  entry,
  messages,
}: {
  entry: WorkerSessionTimelineEntry;
  messages: WorkerSessionTimelineMessages;
}) {
  return (
    <DetailSection heading={messages.sourceLabel}>
      <DetailList
        items={[
          {
            label: messages.eventPositionLabel(entry.canonical.position),
            value: String(entry.canonical.position),
          },
          {
            label: messages.sourceLabel,
            value: `${entry.canonical.sourceType} / ${entry.canonical.sourceId}`,
          },
          {
            label: messages.sourceSequenceLabel(entry.canonical.sourceSequence),
            value: String(entry.canonical.sourceSequence),
          },
          {
            label: messages.genericSchemaLabel(entry.canonical.schemaId),
            value: entry.canonical.schemaId,
          },
        ]}
      />
    </DetailSection>
  );
}

function contentBlockKey(block: WorkerTimelineContentBlock): string {
  return [
    block.kind,
    block.toolCallId ?? "",
    block.toolName ?? "",
    block.text ?? "",
  ].join(":");
}
