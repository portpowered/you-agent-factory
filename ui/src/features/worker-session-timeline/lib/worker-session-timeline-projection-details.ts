import {
  asObject,
  booleanValue,
  copyJSONValue,
  finiteNumber,
  firstString,
  hasKeys,
  optionalString,
} from "./worker-session-timeline-projection-helpers";
import type {
  WorkerTimelineAttempt,
  WorkerTimelineContentBlock,
  WorkerTimelineContinuation,
  WorkerTimelineEntryCategory,
  WorkerTimelineFailure,
  WorkerTimelineIdentity,
  WorkerTimelineJSONObject,
  WorkerTimelineMessage,
  WorkerTimelineMessageDelta,
  WorkerTimelineProgress,
  WorkerTimelineProviderSession,
  WorkerTimelineReasoning,
  WorkerTimelineTerminalOutcome,
  WorkerTimelineTool,
  WorkerTimelineUsage,
} from "./worker-session-timeline-projection-types";

export function categoryForKind(kind: string): WorkerTimelineEntryCategory {
  switch (kind) {
    case "SESSION":
      return "session";
    case "RUN":
      return "run";
    case "TURN":
      return "turn";
    case "MESSAGE":
      return "message";
    case "REASONING":
      return "reasoning";
    case "TOOL":
      return "tool";
    case "FILE_CHANGE":
      return "file-change";
    case "PLAN":
      return "plan";
    case "PROGRESS":
      return "progress";
    case "USAGE":
      return "usage";
    case "ERROR":
      return "error";
    case "STREAM_GAP":
      return "stream-gap";
    default:
      return "generic";
  }
}

export function projectIdentity(
  envelope: WorkerTimelineJSONObject | undefined,
  payload: WorkerTimelineJSONObject,
  usage: WorkerTimelineUsage | undefined,
): WorkerTimelineIdentity | undefined {
  const provenance = asObject(envelope?.provenance);
  const providerSelection = asObject(payload.providerSelection);
  const modelProvider = firstString(
    providerSelection?.modelProvider,
    payload.modelProvider,
  );
  const executorProvider = firstString(
    providerSelection?.executorProvider,
    payload.executorProvider,
  );
  const provider = firstString(
    payload.provider,
    provenance?.provider,
    modelProvider,
    executorProvider,
  );
  const model = firstString(payload.model, usage?.model);
  const identity: WorkerTimelineIdentity = {
    ...(provider !== undefined ? { provider } : {}),
    ...(modelProvider !== undefined ? { modelProvider } : {}),
    ...(executorProvider !== undefined ? { executorProvider } : {}),
    ...(model !== undefined ? { model } : {}),
  };
  return hasKeys(identity) ? identity : undefined;
}

export function projectAttempt(
  envelope: WorkerTimelineJSONObject | undefined,
  payload: WorkerTimelineJSONObject,
): WorkerTimelineAttempt | undefined {
  const id = firstString(
    payload.attemptId,
    envelope?.attemptId,
    envelope?.dispatchId,
  );
  const number = finiteNumber(payload.attempt);
  const reason = firstString(
    payload.attemptReason,
    payload.retryReason,
    payload.reason,
  );
  const dispatchId = firstString(payload.dispatchId, envelope?.dispatchId);
  const turnId = firstString(payload.turnId, envelope?.turnId);
  const status = optionalString(payload.status);
  const attempt: WorkerTimelineAttempt = {
    ...(id !== undefined ? { id } : {}),
    ...(number !== undefined ? { number } : {}),
    ...(reason !== undefined ? { reason } : {}),
    ...(dispatchId !== undefined ? { dispatchId } : {}),
    ...(turnId !== undefined ? { turnId } : {}),
    ...(status !== undefined ? { status } : {}),
  };
  return hasKeys(attempt) ? attempt : undefined;
}

export function projectContinuation(
  payload: WorkerTimelineJSONObject,
): WorkerTimelineContinuation | undefined {
  const continuation = asObject(payload.continuation);
  const lineage = asObject(payload.lineage);
  const providerSession = projectProviderSession(continuation);
  const predecessorWorkerSessionId = firstString(
    lineage?.predecessorWorkerSessionId,
    payload.predecessorWorkerSessionId,
  );
  const successorWorkerSessionId = firstString(
    lineage?.successorWorkerSessionId,
    payload.successorWorkerSessionId,
  );
  const previousDispatchId = firstString(
    lineage?.previousDispatchId,
    payload.previousDispatchId,
  );
  const previousAttemptId = firstString(
    lineage?.previousAttemptId,
    payload.previousAttemptId,
  );
  const links: WorkerTimelineContinuation = {
    ...(providerSession !== undefined ? { providerSession } : {}),
    ...(predecessorWorkerSessionId !== undefined
      ? { predecessorWorkerSessionId }
      : {}),
    ...(successorWorkerSessionId !== undefined
      ? { successorWorkerSessionId }
      : {}),
    ...(previousDispatchId !== undefined ? { previousDispatchId } : {}),
    ...(previousAttemptId !== undefined ? { previousAttemptId } : {}),
  };
  return hasKeys(links) ? links : undefined;
}

function projectProviderSession(
  value: WorkerTimelineJSONObject | undefined,
): WorkerTimelineProviderSession | undefined {
  const provider = optionalString(value?.provider);
  const kind = optionalString(value?.kind);
  const id = optionalString(value?.id);
  if (provider === undefined || kind === undefined || id === undefined) {
    return undefined;
  }
  return { provider, kind, id };
}

export function projectMessage(
  payload: WorkerTimelineJSONObject,
): WorkerTimelineMessage | undefined {
  const role = optionalString(payload.role);
  const partial = booleanValue(payload.partial);
  const contentBlocks = projectContentBlocks(payload.contentBlocks);
  const delta = projectMessageDelta(payload);
  const text = contentBlocks
    ?.map((block) => block.text ?? "")
    .filter((blockText) => blockText.length > 0)
    .join("");
  const message: WorkerTimelineMessage = {
    ...(role !== undefined ? { role } : {}),
    ...(contentBlocks !== undefined ? { contentBlocks } : {}),
    ...(text !== undefined && text.length > 0 ? { text } : {}),
    ...(partial !== undefined ? { partial } : {}),
    ...(delta !== undefined ? { delta } : {}),
  };
  return hasKeys(message) ? message : undefined;
}

function projectMessageDelta(
  payload: WorkerTimelineJSONObject,
): WorkerTimelineMessageDelta | undefined {
  const contentBlockIndex = finiteNumber(payload.contentBlockIndex);
  const contentBlockKind = optionalString(payload.contentBlockKind);
  const textDelta = optionalString(payload.textDelta);
  const delta: WorkerTimelineMessageDelta = {
    ...(contentBlockIndex !== undefined ? { contentBlockIndex } : {}),
    ...(contentBlockKind !== undefined ? { contentBlockKind } : {}),
    ...(textDelta !== undefined ? { textDelta } : {}),
  };
  return hasKeys(delta) ? delta : undefined;
}

function projectContentBlocks(
  value: unknown,
): WorkerTimelineContentBlock[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const blocks = value
    .map((candidate) => {
      const block = asObject(candidate);
      const kind = optionalString(block?.kind);
      const text = optionalString(block?.text);
      const toolCallId = optionalString(block?.toolCallId);
      const toolName = optionalString(block?.toolName);
      const imageRef = optionalString(block?.imageRef);
      const resourceRef = optionalString(block?.resourceRef);
      const argumentsSummary = copyJSONValue(block?.argumentsSummary);
      const structuredOutput = copyJSONValue(block?.structuredOutput);
      if (kind === undefined) {
        return undefined;
      }
      return {
        kind,
        ...(text !== undefined ? { text } : {}),
        ...(toolCallId !== undefined ? { toolCallId } : {}),
        ...(toolName !== undefined ? { toolName } : {}),
        ...(argumentsSummary !== undefined ? { argumentsSummary } : {}),
        ...(imageRef !== undefined ? { imageRef } : {}),
        ...(resourceRef !== undefined ? { resourceRef } : {}),
        ...(structuredOutput !== undefined ? { structuredOutput } : {}),
      } satisfies WorkerTimelineContentBlock;
    })
    .filter(
      (block): block is WorkerTimelineContentBlock => block !== undefined,
    );
  return blocks;
}

export function projectReasoning(
  phase: string,
  payload: WorkerTimelineJSONObject,
): WorkerTimelineReasoning | undefined {
  const summary = optionalString(payload.summary);
  const summaryDelta = optionalString(payload.summaryDelta);
  const reasoning: WorkerTimelineReasoning = {
    ...(summary !== undefined ? { summary } : {}),
    ...(summaryDelta !== undefined ? { summaryDelta } : {}),
    ...(summaryDelta !== undefined || phase === "DELTA"
      ? { representation: "DELTA" }
      : summary !== undefined
        ? { representation: "SNAPSHOT" }
        : {}),
  };
  return hasKeys(reasoning) ? reasoning : undefined;
}

export function projectProgress(
  payload: WorkerTimelineJSONObject,
): WorkerTimelineProgress | undefined {
  const label = optionalString(payload.label);
  const message = optionalString(payload.message);
  const percentComplete = finiteNumber(payload.percentComplete);
  const progress: WorkerTimelineProgress = {
    ...(label !== undefined ? { label } : {}),
    ...(message !== undefined ? { message } : {}),
    ...(percentComplete !== undefined ? { percentComplete } : {}),
  };
  return hasKeys(progress) ? progress : undefined;
}

export function projectTool(
  payload: WorkerTimelineJSONObject,
): WorkerTimelineTool | undefined {
  const toolCallId = optionalString(payload.toolCallId);
  const toolName = optionalString(payload.toolName);
  const status = optionalString(payload.status);
  const outputDelta = optionalString(payload.outputDelta);
  const argumentsSummary = copyJSONValue(payload.argumentsSummary);
  const resultSummary = copyJSONValue(payload.resultSummary);
  const tool: WorkerTimelineTool = {
    ...(toolCallId !== undefined ? { toolCallId } : {}),
    ...(toolName !== undefined ? { toolName } : {}),
    ...(status !== undefined ? { status } : {}),
    ...(argumentsSummary !== undefined ? { argumentsSummary } : {}),
    ...(resultSummary !== undefined ? { resultSummary } : {}),
    ...(outputDelta !== undefined ? { outputDelta } : {}),
  };
  return hasKeys(tool) ? tool : undefined;
}

export function projectUsage(
  payload: WorkerTimelineJSONObject,
): WorkerTimelineUsage | undefined {
  const inputTokens = finiteNumber(payload.inputTokens);
  const cachedInputTokens = finiteNumber(payload.cachedInputTokens);
  const cacheWriteTokens = finiteNumber(payload.cacheWriteTokens);
  const outputTokens = finiteNumber(payload.outputTokens);
  const reasoningOutputTokens = finiteNumber(payload.reasoningOutputTokens);
  const totalTokens = finiteNumber(payload.totalTokens);
  const model = optionalString(payload.model);
  const usage: WorkerTimelineUsage = {
    ...(inputTokens !== undefined ? { inputTokens } : {}),
    ...(cachedInputTokens !== undefined ? { cachedInputTokens } : {}),
    ...(cacheWriteTokens !== undefined ? { cacheWriteTokens } : {}),
    ...(outputTokens !== undefined ? { outputTokens } : {}),
    ...(reasoningOutputTokens !== undefined ? { reasoningOutputTokens } : {}),
    ...(totalTokens !== undefined ? { totalTokens } : {}),
    ...(model !== undefined ? { model } : {}),
  };
  return hasKeys(usage) ? usage : undefined;
}

export function projectFailure(
  kind: string,
  payload: WorkerTimelineJSONObject,
): WorkerTimelineFailure | undefined {
  const failureKind = firstString(
    payload.kind,
    payload.failureKind,
    payload.failureCause,
  );
  const code = firstString(
    payload.code,
    payload.errorCode,
    payload.agentRunFailureClass,
  );
  const message = firstString(
    payload.message,
    payload.failureDetail,
    payload.errorMessage,
  );
  const retryable = booleanValue(payload.retryable);
  const retryAfterSeconds = finiteNumber(payload.retryAfterSeconds);
  const retryAttempt = finiteNumber(payload.retryAttempt);
  const failure: WorkerTimelineFailure = {
    ...(failureKind !== undefined ? { kind: failureKind } : {}),
    ...(code !== undefined ? { code } : {}),
    ...(message !== undefined ? { message } : {}),
    ...(retryable !== undefined ? { retryable } : {}),
    ...(retryAfterSeconds !== undefined ? { retryAfterSeconds } : {}),
    ...(retryAttempt !== undefined ? { retryAttempt } : {}),
  };
  if (kind !== "ERROR" && !hasKeys(failure)) {
    return undefined;
  }
  return hasKeys(failure) ? failure : undefined;
}

export function terminalOutcomeFor(
  kind: string,
  phase: string,
  payload: WorkerTimelineJSONObject,
): WorkerTimelineTerminalOutcome | undefined {
  if (kind !== "SESSION" && kind !== "RUN" && kind !== "TURN") {
    return undefined;
  }
  switch (phase) {
    case "COMPLETED":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "SUCCESS";
    case "FAILED":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "FAILURE";
    case "CANCELED":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "CANCELED";
    default:
      break;
  }
  switch (
    String(payload.status ?? "")
      .trim()
      .toUpperCase()
  ) {
    case "COMPLETED":
    case "SUCCEEDED":
    case "SUCCESS":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "SUCCESS";
    case "FAILED":
    case "FAILURE":
    case "ERROR":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "FAILURE";
    case "CANCELED":
    case "CANCELLED":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "CANCELED";
    default:
      return undefined;
  }
}
