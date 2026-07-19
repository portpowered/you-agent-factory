import {
  type FactoryRecording,
  type RecordingValidationIssue,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";
import { Text } from "@you-agent-factory/components";
import {
  canonicalizeFactoryEvents,
  type FactoryActivityProjection,
  type FactoryLoadProjection,
  type FactoryTopologyNode,
  type FactoryTopologyProjection,
  type FactoryWorkProgressProjection,
  projectFactoryActivityAtTick,
  projectFactoryLoadAtTick,
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgressAtTick,
} from "@you-agent-factory/factory-replay";
import { useEffect, useMemo, useRef, useState } from "react";

import {
  type FactoryTimelineMode,
  FactoryTimelineScrubber,
  type FactoryTimelineScrubberMessages,
} from "./factory-timeline-scrubber";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
} from "./factory-topology-replay";
import type { FactoryVisualizerError } from "./visualizer-error";
import { factoryVisualizerErrorKey } from "./visualizer-error";
import {
  WorkProgressVisualizer,
  type WorkProgressVisualizerMessages,
} from "./work-progress-visualizer";

export interface FactoryRecordingValidationDiagnosticIssue {
  category: RecordingValidationIssue["category"];
  code: RecordingValidationIssue["code"];
  path: readonly (number | string)[];
}

export interface FactoryRecordingValidationDiagnostic {
  issues: readonly FactoryRecordingValidationDiagnosticIssue[];
  kind: "recording-validation";
  message: string;
  recoverable: false;
}

export type FactoryRecordingTopologyReplayError =
  | FactoryRecordingValidationDiagnostic
  | FactoryVisualizerError
  | import("./visualizer-error").FactoryVisualizationLayoutDiagnostic;

export interface FactoryRecordingTopologyReplayMessages {
  progress: WorkProgressVisualizerMessages;
  regionLabel: string;
  selectedTick: (formattedTick: string) => string;
  timeline: FactoryTimelineScrubberMessages;
  topology: FactoryTopologyReplayMessages;
  validationFailed: string;
}

export interface FactoryRecordingTopologyReplayProps {
  defaultSelectedTick?: number;
  formatNumber: (value: number) => string;
  /** Presentation-only content validated by the controlled topology renderer. */
  layout?: unknown;
  messages: FactoryRecordingTopologyReplayMessages;
  onError?: (error: FactoryRecordingTopologyReplayError) => void;
  onSelectNode?: (node: FactoryTopologyNode) => void;
  recording?: unknown;
  selectedNodeId?: string;
  state?: FactoryRecordingTopologyReplayState;
}

export type FactoryRecordingTopologyReplayState =
  | { error: FactoryVisualizerError; status: "failed" }
  | { status: "loading" }
  | { recording: unknown; status: "ready" };

interface RecordingProjection {
  activity: FactoryActivityProjection;
  load: FactoryLoadProjection;
  progress: FactoryWorkProgressProjection;
  topology: FactoryTopologyProjection;
}

const MAX_CACHED_RECORDING_PROJECTIONS = 32;

/** Validate and replay one caller-owned recording through the controlled visualizers. */
export function FactoryRecordingTopologyReplay({
  defaultSelectedTick,
  formatNumber,
  layout,
  messages,
  onError,
  onSelectNode,
  recording,
  selectedNodeId,
  state,
}: FactoryRecordingTopologyReplayProps) {
  const status = state?.status ?? "ready";
  const recordingInput =
    state?.status === "ready" ? state.recording : recording;
  const parsed = useMemo(
    () =>
      status === "ready"
        ? safeParseFactoryRecording(recordingInput)
        : undefined,
    [recordingInput, status],
  );
  const validationError = useMemo(
    () =>
      !parsed || parsed.success
        ? undefined
        : toRecordingValidationDiagnostic(
            parsed.issues,
            messages.validationFailed,
          ),
    [messages.validationFailed, parsed],
  );
  useDistinctRecordingErrorReport(validationError, onError);
  useDistinctVisualizerErrorReport(
    state?.status === "failed" ? state.error : undefined,
    onError,
  );

  if (status === "loading" || status === "failed") {
    return (
      <FactoryTopologyReplay messages={messages.topology} state={{ status }} />
    );
  }

  if (!parsed?.success) {
    return (
      <FactoryTopologyReplay
        messages={messages.topology}
        onError={onError}
        state={{ status: "failed" }}
      />
    );
  }

  return (
    <ValidatedRecordingReplay
      defaultSelectedTick={defaultSelectedTick}
      formatNumber={formatNumber}
      key={parsed.data.id}
      layout={layout}
      messages={messages}
      onError={onError}
      onSelectNode={onSelectNode}
      recording={parsed.data}
      selectedNodeId={selectedNodeId}
    />
  );
}

function ValidatedRecordingReplay({
  defaultSelectedTick,
  formatNumber,
  layout,
  messages,
  onError,
  onSelectNode,
  recording,
  selectedNodeId,
}: Omit<FactoryRecordingTopologyReplayProps, "recording"> & {
  recording: FactoryRecording;
}) {
  const events = useMemo(
    () => canonicalizeFactoryEvents(recording.events),
    [recording.events],
  );
  const ticks = useMemo(() => recordedTicks(events), [events]);
  const latestTick = ticks.at(-1) ?? 0;
  const defaultTick =
    defaultSelectedTick !== undefined && ticks.includes(defaultSelectedTick)
      ? defaultSelectedTick
      : latestTick;
  const [mode, setMode] = useState<FactoryTimelineMode>(() =>
    defaultSelectedTick !== undefined && ticks.includes(defaultSelectedTick)
      ? "history"
      : "current",
  );
  const [fixedTick, setFixedTick] = useState(defaultTick);
  const effectiveMode =
    mode === "history" && ticks.includes(fixedTick) ? "history" : "current";
  const selectedTick = effectiveMode === "current" ? latestTick : fixedTick;
  const projectionCache = useRef(new Map<string, RecordingProjection>());
  const evidenceKey = JSON.stringify(
    events.filter((event) => event.context.tick <= selectedTick),
  );
  const prepared = useMemo(
    () =>
      projectRecordingAtTick(
        events,
        selectedTick,
        evidenceKey,
        projectionCache.current,
      ),
    [events, evidenceKey, selectedTick],
  );

  function selectTick(requestedTick: number) {
    const resolvedTick = resolveRecordedTick(
      ticks,
      selectedTick,
      requestedTick,
    );
    setFixedTick(resolvedTick);
    setMode("history");
  }

  function followLatest() {
    setMode("current");
  }

  return (
    <section
      aria-label={messages.regionLabel}
      className="factory-recording-topology-replay"
      data-selected-tick={selectedTick}
    >
      <Text as="p" className="factory-recording-topology-replay__tick">
        {messages.selectedTick(formatNumber(selectedTick))}
      </Text>
      <FactoryTimelineScrubber
        formatTick={formatNumber}
        messages={messages.timeline}
        onFollowLatest={followLatest}
        onSelectTick={selectTick}
        state={{
          earliestTick: ticks[0] ?? 0,
          latestTick,
          mode: effectiveMode,
          selectedTick,
          status: "available",
        }}
      />
      <FactoryTopologyReplay
        layout={layout}
        messages={messages.topology}
        onError={onError}
        onSelectNode={onSelectNode}
        selectedNodeId={selectedNodeId}
        state={{
          ...(prepared.topology.nodes.length === 0
            ? { status: "empty" as const }
            : {
                projection: {
                  activity: prepared.activity,
                  load: prepared.load,
                  topology: prepared.topology,
                },
                status: "ready" as const,
              }),
        }}
      />
      <WorkProgressVisualizer
        formatNumber={formatNumber}
        messages={messages.progress}
        projection={prepared.progress}
      />
    </section>
  );
}

function recordedTicks(events: FactoryRecording["events"]): number[] {
  const ticks = [...new Set(events.map((event) => event.context.tick))];
  return ticks.length > 0 ? ticks : [0];
}

function resolveRecordedTick(
  ticks: readonly number[],
  selectedTick: number,
  requestedTick: number,
): number {
  if (ticks.includes(requestedTick)) return requestedTick;
  if (requestedTick > selectedTick) {
    return ticks.find((tick) => tick >= requestedTick) ?? ticks.at(-1) ?? 0;
  }
  return (
    [...ticks].reverse().find((tick) => tick <= requestedTick) ?? ticks[0] ?? 0
  );
}

function projectRecordingAtTick(
  events: FactoryRecording["events"],
  tick: number,
  evidenceKey: string,
  cache: Map<string, RecordingProjection>,
): RecordingProjection {
  const cacheKey = `${tick}:${evidenceKey}`;
  const cached = cache.get(cacheKey);
  if (cached) return cached;

  const projection = {
    activity: projectFactoryActivityAtTick({ events, tick }),
    load: projectFactoryLoadAtTick({ events, tick }),
    progress: projectFactoryWorkProgressAtTick({ events, tick }),
    topology: projectFactoryTopologyAtTick({ events, tick }),
  };
  cache.set(cacheKey, projection);
  if (cache.size > MAX_CACHED_RECORDING_PROJECTIONS) {
    const oldestKey = cache.keys().next().value;
    if (oldestKey !== undefined) cache.delete(oldestKey);
  }
  return projection;
}

function toRecordingValidationDiagnostic(
  issues: readonly RecordingValidationIssue[],
  message: string,
): FactoryRecordingValidationDiagnostic {
  return {
    issues: issues.map(({ category, code, path }) => ({
      category,
      code,
      path,
    })),
    kind: "recording-validation",
    message,
    recoverable: false,
  };
}

function useDistinctRecordingErrorReport(
  error: FactoryRecordingValidationDiagnostic | undefined,
  onError: FactoryRecordingTopologyReplayProps["onError"],
) {
  const reported = useRef(new Set<string>());
  useEffect(() => {
    if (!error) return;
    const key = error.issues
      .map((issue) => `${issue.category}:${issue.code}:${issue.path.join(".")}`)
      .join("|");
    if (reported.current.has(key)) return;
    reported.current.add(key);
    onError?.(error);
  }, [error, onError]);
}

function useDistinctVisualizerErrorReport(
  error: FactoryVisualizerError | undefined,
  onError: FactoryRecordingTopologyReplayProps["onError"],
) {
  const reported = useRef(new Set<string>());
  useEffect(() => {
    if (!error) return;
    const key = factoryVisualizerErrorKey(error);
    if (reported.current.has(key)) return;
    reported.current.add(key);
    onError?.(error);
  }, [error, onError]);
}
