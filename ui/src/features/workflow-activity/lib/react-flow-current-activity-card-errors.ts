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

export type CurrentActivityReactFlowErrorClassification =
  | "recoverable"
  | "integrity";

/**
 * React Flow reports both transient setup diagnostics and unsafe graph
 * integrity failures through the same callback. Keep the known transient
 * diagnostics non-fatal so they cannot take down the graph card; unknown
 * diagnostics remain fatal until they are deliberately classified.
 */
const RECOVERABLE_REACT_FLOW_ERROR_IDS = new Set([
  "002", // nodeTypes/edgeTypes identity warning
  "003", // missing node type falls back to the default renderer
  "004", // parent measurement is not ready yet
  "005", // parent extent was supplied for a non-child node
  "007", // an edge disappeared during an interaction
  "009", // marker type falls back to the default renderer
  "011", // edge type falls back to the default renderer
  "012", // node disappeared before a delayed interaction callback
  "013", // styles were not loaded
  "015", // node measurement is not ready for a drag
  "016", // edge disappeared before a delayed interaction callback
]);

export function classifyCurrentActivityReactFlowError(
  errorId: string,
): CurrentActivityReactFlowErrorClassification {
  return RECOVERABLE_REACT_FLOW_ERROR_IDS.has(errorId)
    ? "recoverable"
    : "integrity";
}

export function isFatalCurrentActivityReactFlowError(errorId: string): boolean {
  return classifyCurrentActivityReactFlowError(errorId) === "integrity";
}

export function handleCurrentActivityReactFlowError(
  errorId: string,
  message: string,
): void {
  if (!isFatalCurrentActivityReactFlowError(errorId)) {
    return;
  }

  throw new CurrentActivityGraphEndpointError(errorId, message);
}
