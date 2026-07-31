export type FactoryVisualizerErrorKind =
  | "endpoint"
  | "layout"
  | "projection"
  | "react-flow"
  | "render";

export interface FactoryVisualizerErrorCause {
  code?: string;
  name: string;
}

export interface FactoryVisualizerError {
  cause?: FactoryVisualizerErrorCause;
  kind: FactoryVisualizerErrorKind;
  message: string;
  recoverable: boolean;
}

export type FactoryTopologyReplayError = FactoryVisualizerError;

const ERROR_MESSAGES: Record<FactoryVisualizerErrorKind, string> = {
  endpoint: "The prepared topology contains invalid edge endpoints.",
  layout: "The topology layout could not be prepared.",
  projection: "The prepared topology projection could not be read.",
  "react-flow": "The topology renderer failed.",
  render: "The visualizer could not render.",
};

export class FactoryVisualizerInternalError extends Error {
  readonly kind: FactoryVisualizerErrorKind;
  readonly originalCause: unknown;

  constructor(kind: FactoryVisualizerErrorKind, cause?: unknown) {
    super(ERROR_MESSAGES[kind]);
    this.name = "FactoryVisualizerInternalError";
    this.kind = kind;
    this.originalCause = cause;
  }
}

export function toFactoryVisualizerError(
  kind: FactoryVisualizerErrorKind,
  cause?: unknown,
): FactoryVisualizerError {
  const safeCause = sanitizeCause(cause);
  return {
    ...(safeCause ? { cause: safeCause } : {}),
    kind,
    message: ERROR_MESSAGES[kind],
    recoverable: true,
  };
}

export function normalizeFactoryVisualizerError(
  error: unknown,
  fallbackKind: FactoryVisualizerErrorKind,
): FactoryVisualizerError {
  if (error instanceof FactoryVisualizerInternalError) {
    return toFactoryVisualizerError(error.kind, error.originalCause);
  }
  return toFactoryVisualizerError(fallbackKind, error);
}

export function factoryVisualizerErrorKey(
  error: FactoryVisualizerError,
): string {
  return [
    error.kind,
    error.message,
    error.recoverable ? "recoverable" : "terminal",
    error.cause?.name ?? "",
    error.cause?.code ?? "",
  ].join(":");
}

function sanitizeCause(
  cause: unknown,
): FactoryVisualizerErrorCause | undefined {
  if (!(cause instanceof Error)) return undefined;
  const code = readSafeCode(cause);
  return {
    ...(code ? { code } : {}),
    name: safeErrorName(cause.name),
  };
}

function readSafeCode(error: Error): string | undefined {
  const code = (error as Error & { code?: unknown }).code;
  return typeof code === "string" && /^[A-Z0-9_-]{1,64}$/.test(code)
    ? code
    : undefined;
}

function safeErrorName(name: string): string {
  return /^[A-Za-z][A-Za-z0-9]*Error$/.test(name) ? name : "Error";
}
