import {
  CurrentActivityGraphEndpointError,
  handleCurrentActivityReactFlowError,
  isFatalCurrentActivityReactFlowError,
} from "./react-flow-current-activity-card-errors";

describe("current activity React Flow error handling", () => {
  it("classifies React Flow endpoint errors as fatal", () => {
    expect(isFatalCurrentActivityReactFlowError("006")).toBe(true);
    expect(isFatalCurrentActivityReactFlowError("008")).toBe(true);
    expect(isFatalCurrentActivityReactFlowError("004")).toBe(true);
  });

  it("throws endpoint errors with graph handle context", () => {
    expect(() =>
      handleCurrentActivityReactFlowError(
        "008",
        'Couldn\'t create edge for source handle id: "missing-review-source", edge id: e1.',
      ),
    ).toThrow(CurrentActivityGraphEndpointError);

    expect(() =>
      handleCurrentActivityReactFlowError(
        "008",
        'Couldn\'t create edge for source handle id: "missing-review-source", edge id: e1.',
      ),
    ).toThrow(
      /sourceHandle and targetHandle values match rendered node handles/,
    );
  });

  it("throws unrecognized React Flow errors instead of warning", () => {
    expect(() =>
      handleCurrentActivityReactFlowError(
        "004",
        "The React Flow parent container needs a width and a height to render the graph.",
      ),
    ).toThrow(CurrentActivityGraphEndpointError);
  });
});
