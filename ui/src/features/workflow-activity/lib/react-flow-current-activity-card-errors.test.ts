import {
  CurrentActivityGraphEndpointError,
  handleCurrentActivityReactFlowError,
  isFatalCurrentActivityReactFlowError,
} from "./react-flow-current-activity-card-errors";

describe("current activity React Flow error handling", () => {
  it("classifies React Flow endpoint errors as fatal", () => {
    expect(isFatalCurrentActivityReactFlowError("006")).toBe(true);
    expect(isFatalCurrentActivityReactFlowError("008")).toBe(true);
    expect(isFatalCurrentActivityReactFlowError("004")).toBe(false);
  });

  it("throws endpoint errors with graph handle context", () => {
    expect(() =>
      handleCurrentActivityReactFlowError(
        "008",
        'Couldn\'t create edge for source handle id: "out-review", edge id: e1.',
      ),
    ).toThrow(CurrentActivityGraphEndpointError);

    expect(() =>
      handleCurrentActivityReactFlowError(
        "008",
        'Couldn\'t create edge for source handle id: "out-review", edge id: e1.',
      ),
    ).toThrow(/sourceHandle and targetHandle values match rendered node handles/);
  });

  it("keeps unrecognized React Flow errors on the warning path", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);

    expect(() =>
      handleCurrentActivityReactFlowError(
        "004",
        "The React Flow parent container needs a width and a height to render the graph.",
      ),
    ).not.toThrow();

    expect(warn).toHaveBeenCalledWith(
      "[React Flow]: The React Flow parent container needs a width and a height to render the graph. Help: https://reactflow.dev/error#004",
    );
  });
});
