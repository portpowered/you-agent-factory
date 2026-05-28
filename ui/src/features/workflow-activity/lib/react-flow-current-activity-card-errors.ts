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

export function isFatalCurrentActivityReactFlowError(_errorId: string) {
  return true;
}

export function handleCurrentActivityReactFlowError(
  errorId: string,
  message: string,
): void {
  throw new CurrentActivityGraphEndpointError(errorId, message);
}
