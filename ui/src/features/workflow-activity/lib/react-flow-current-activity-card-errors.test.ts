import {
  CurrentActivityGraphEndpointError,
  classifyCurrentActivityReactFlowError,
  handleCurrentActivityReactFlowError,
  isFatalCurrentActivityReactFlowError,
} from "./react-flow-current-activity-card-errors";

describe("current activity React Flow error handling", () => {
  it("classifies known transient diagnostics as recoverable", () => {
    expect(classifyCurrentActivityReactFlowError("004")).toBe("recoverable");
    expect(classifyCurrentActivityReactFlowError("015")).toBe("recoverable");
    expect(isFatalCurrentActivityReactFlowError("004")).toBe(false);
    expect(isFatalCurrentActivityReactFlowError("015")).toBe(false);
  });

  it("classifies graph-integrity diagnostics and unknown diagnostics as fatal", () => {
    expect(classifyCurrentActivityReactFlowError("006")).toBe("integrity");
    expect(classifyCurrentActivityReactFlowError("008")).toBe("integrity");
    expect(classifyCurrentActivityReactFlowError("unexpected")).toBe(
      "integrity",
    );
    expect(isFatalCurrentActivityReactFlowError("006")).toBe(true);
    expect(isFatalCurrentActivityReactFlowError("008")).toBe(true);
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

  it("does not throw recoverable React Flow errors", () => {
    expect(() =>
      handleCurrentActivityReactFlowError(
        "004",
        "The React Flow parent container needs a width and a height to render the graph.",
      ),
    ).not.toThrow();
  });

  it("throws unclassified React Flow errors instead of allowing unsafe rendering", () => {
    expect(() =>
      handleCurrentActivityReactFlowError(
        "unexpected",
        "An unclassified renderer failure occurred.",
      ),
    ).toThrow(CurrentActivityGraphEndpointError);
  });
});
