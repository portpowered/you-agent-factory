import type { FactoryRecording } from "./contracts.js";

export type FactoryRecordingValidationIssueCode =
  | "INVALID_SHAPE"
  | "UNSUPPORTED_RECORDING_VERSION"
  | "UNSUPPORTED_EVENT_VERSION"
  | "DUPLICATE_EVENT_ID"
  | "MIXED_SESSION_ID"
  | "NON_CANONICAL_ORDER"
  | "MISSING_TOPOLOGY_BOOTSTRAP";

export interface FactoryRecordingValidationIssue {
  code: FactoryRecordingValidationIssueCode;
  path: string;
  message: string;
  eventIndex?: number;
  eventId?: string;
}

export class FactoryRecordingValidationError extends Error {
  readonly issues: readonly FactoryRecordingValidationIssue[];
  constructor(issues: readonly FactoryRecordingValidationIssue[]);
}

export type FactoryRecordingParseResult =
  | { success: true; data: FactoryRecording }
  | {
      success: false;
      error: FactoryRecordingValidationError;
      issues: readonly FactoryRecordingValidationIssue[];
    };

export function safeParseFactoryRecording(
  input: unknown,
): FactoryRecordingParseResult;

export function parseFactoryRecording(input: unknown): FactoryRecording;
