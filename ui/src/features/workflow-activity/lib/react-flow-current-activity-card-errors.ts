const REACT_FLOW_FATAL_ENDPOINT_ERROR_IDS = new Set(["006", "008"]);

export class CurrentActivityGraphEndpointError extends Error {
  public readonly reactFlowErrorId: string;

  constructor(errorId: string, message: string) {
    super(
      `React Flow factory graph endpoint error ${errorId}: ${message} Check that edge sourceHandle and targetHandle values match rendered node handles.`,
    );
    this.name = "CurrentActivityGraphEndpointError";
    this.reactFlowErrorId = errorId;
  }
}

export function isFatalCurrentActivityReactFlowError(errorId: string) {
  return REACT_FLOW_FATAL_ENDPOINT_ERROR_IDS.has(errorId);
}

export function handleCurrentActivityReactFlowError(
  errorId: string,
  message: string,
): void {
  if (isFatalCurrentActivityReactFlowError(errorId)) {
    throw new CurrentActivityGraphEndpointError(errorId, message);
  }

  console.warn(
    `[React Flow]: ${message} Help: https://reactflow.dev/error#${errorId}`,
  );
}
