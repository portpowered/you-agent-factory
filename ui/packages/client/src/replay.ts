import type { FactoryEvent } from "./contracts.js";
import { orderFactoryEvents } from "./event-ordering.js";
import {
  type RecordingValidationIssueCode,
  validateFactoryEventEnvelope,
  validateFactoryEventSchemaVersion,
} from "./recording.js";

export type FactoryReplayTextIssueCode =
  | "malformed_event_json"
  | RecordingValidationIssueCode;

export interface FactoryReplayTextIssue {
  code: FactoryReplayTextIssueCode;
  path: readonly (string | number)[];
  message: string;
}

export type SafeParseFactoryEventReplayTextResult =
  | { success: true; data: FactoryEvent[] }
  | { success: false; issues: readonly FactoryReplayTextIssue[] };

export class FactoryReplayTextParseError extends Error {
  readonly issues: readonly FactoryReplayTextIssue[];

  constructor(issues: readonly FactoryReplayTextIssue[]) {
    super(
      issues.length === 1
        ? `Factory replay text parsing failed: ${issues[0]?.message}`
        : `Factory replay text parsing failed with ${issues.length} issues`,
    );
    this.name = "FactoryReplayTextParseError";
    this.issues = issues;
  }
}

function dataFrames(replayText: string): string[] {
  const normalizedText = replayText
    .replaceAll("\r\n", "\n")
    .replaceAll("\r", "\n");
  const frames: string[] = [];

  for (const block of normalizedText.split(/\n[\t ]*\n/)) {
    const dataLines: string[] = [];
    for (const line of block.split("\n")) {
      if (line.startsWith(":")) {
        continue;
      }

      const separator = line.indexOf(":");
      const field = separator === -1 ? line : line.slice(0, separator);
      if (field !== "data") {
        continue;
      }

      let value = separator === -1 ? "" : line.slice(separator + 1);
      if (value.startsWith(" ")) {
        value = value.slice(1);
      }
      dataLines.push(value);
    }

    if (dataLines.length > 0) {
      frames.push(dataLines.join("\n"));
    }
  }

  return frames;
}

export function safeParseFactoryEventReplayText(
  replayText: string,
): SafeParseFactoryEventReplayTextResult {
  const events: FactoryEvent[] = [];
  const issues: FactoryReplayTextIssue[] = [];

  for (const [frameIndex, data] of dataFrames(replayText).entries()) {
    let input: unknown;
    try {
      input = JSON.parse(data);
    } catch {
      issues.push({
        code: "malformed_event_json",
        path: ["frames", frameIndex, "data"],
        message: "Expected SSE data to contain valid Factory event JSON.",
      });
      continue;
    }

    const framePath = ["frames", frameIndex, "data"] as const;
    const validationIssues = [
      ...validateFactoryEventEnvelope(input, framePath),
      ...validateFactoryEventSchemaVersion(input, framePath),
    ];
    if (validationIssues.length > 0) {
      issues.push(
        ...validationIssues.map(({ code, path, message }) => ({
          code,
          path,
          message,
        })),
      );
      continue;
    }

    events.push(input as FactoryEvent);
  }

  return issues.length > 0
    ? { success: false, issues }
    : { success: true, data: orderFactoryEvents(events) };
}

export function parseFactoryEventReplayText(
  replayText: string,
): FactoryEvent[] {
  const result = safeParseFactoryEventReplayText(replayText);
  if (!result.success) {
    throw new FactoryReplayTextParseError(result.issues);
  }
  return result.data;
}
