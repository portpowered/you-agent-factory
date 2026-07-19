import {
  type FactoryRecording,
  type RecordingValidationIssue,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";
import { Text } from "@you-agent-factory/components";
import {
  canonicalizeFactoryEvents,
  type FactoryTopologyNode,
  projectFactoryActivityAtTick,
  projectFactoryLoadAtTick,
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgressAtTick,
} from "@you-agent-factory/factory-replay";
import { useEffect, useMemo, useRef } from "react";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
} from "./factory-topology-replay";
import type { FactoryVisualizerError } from "./visualizer-error";
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
  | FactoryVisualizerError;

export interface FactoryRecordingTopologyReplayMessages {
  progress: WorkProgressVisualizerMessages;
  regionLabel: string;
  selectedTick: (formattedTick: string) => string;
  topology: FactoryTopologyReplayMessages;
  validationFailed: string;
}

export interface FactoryRecordingTopologyReplayProps {
  defaultSelectedTick?: number;
  formatNumber: (value: number) => string;
  messages: FactoryRecordingTopologyReplayMessages;
  onError?: (error: FactoryRecordingTopologyReplayError) => void;
  onSelectNode?: (node: FactoryTopologyNode) => void;
  recording: unknown;
  selectedNodeId?: string;
}

interface PreparedRecording {
  recording: FactoryRecording;
  selectedTick: number;
}

/** Validate and replay one caller-owned recording through the controlled visualizers. */
export function FactoryRecordingTopologyReplay({
  defaultSelectedTick,
  formatNumber,
  messages,
  onError,
  onSelectNode,
  recording,
  selectedNodeId,
}: FactoryRecordingTopologyReplayProps) {
  const parsed = useMemo(
    () => safeParseFactoryRecording(recording),
    [recording],
  );
  const validationError = useMemo(
    () =>
      parsed.success
        ? undefined
        : toRecordingValidationDiagnostic(
            parsed.issues,
            messages.validationFailed,
          ),
    [messages.validationFailed, parsed],
  );
  useDistinctRecordingErrorReport(validationError, onError);

  if (!parsed.success) {
    return (
      <FactoryTopologyReplay
        messages={messages.topology}
        onError={onError}
        state={{ status: "failed" }}
      />
    );
  }

  const prepared = prepareRecording(parsed.data, defaultSelectedTick);
  const events = prepared.recording.events;
  const projection = {
    activity: projectFactoryActivityAtTick({
      events,
      tick: prepared.selectedTick,
    }),
    load: projectFactoryLoadAtTick({ events, tick: prepared.selectedTick }),
    topology: projectFactoryTopologyAtTick({
      events,
      tick: prepared.selectedTick,
    }),
  };
  const progress = projectFactoryWorkProgressAtTick({
    events,
    tick: prepared.selectedTick,
  });

  return (
    <section
      aria-label={messages.regionLabel}
      className="factory-recording-topology-replay"
      data-selected-tick={prepared.selectedTick}
    >
      <Text as="p" className="factory-recording-topology-replay__tick">
        {messages.selectedTick(formatNumber(prepared.selectedTick))}
      </Text>
      <FactoryTopologyReplay
        messages={messages.topology}
        onError={onError}
        onSelectNode={onSelectNode}
        selectedNodeId={selectedNodeId}
        state={{ projection, status: "ready" }}
      />
      <WorkProgressVisualizer
        formatNumber={formatNumber}
        messages={messages.progress}
        projection={progress}
      />
    </section>
  );
}

function prepareRecording(
  recording: FactoryRecording,
  defaultSelectedTick: number | undefined,
): PreparedRecording {
  const events = canonicalizeFactoryEvents(recording.events);
  const ticks = new Set(events.map((event) => event.context.tick));
  const latestTick = events.at(-1)?.context.tick ?? 0;
  const selectedTick =
    defaultSelectedTick !== undefined && ticks.has(defaultSelectedTick)
      ? defaultSelectedTick
      : latestTick;
  return { recording, selectedTick };
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
