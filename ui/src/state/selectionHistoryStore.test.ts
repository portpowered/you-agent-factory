import {
  resetSelectionHistoryStore,
  useSelectionHistoryStore,
} from "./selectionHistoryStore";

describe("selectionHistoryStore", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("pushes new selections onto the past stack and supports undo and redo", () => {
    const store = useSelectionHistoryStore.getState();

    store.commitSelectionState({
      selection: { kind: "node", nodeId: "review" },
      terminalWorkDetail: null,
    });
    store.commitSelectionState({
      selection: { kind: "state-node", placeId: "story:complete" },
      terminalWorkDetail: null,
    });

    expect(useSelectionHistoryStore.getState().past).toHaveLength(2);
    expect(useSelectionHistoryStore.getState().present.selection).toEqual({
      kind: "state-node",
      placeId: "story:complete",
    });

    useSelectionHistoryStore.getState().undo();

    expect(useSelectionHistoryStore.getState().present.selection).toEqual({
      kind: "node",
      nodeId: "review",
    });
    expect(useSelectionHistoryStore.getState().future).toHaveLength(1);

    useSelectionHistoryStore.getState().redo();

    expect(useSelectionHistoryStore.getState().present.selection).toEqual({
      kind: "state-node",
      placeId: "story:complete",
    });
    expect(useSelectionHistoryStore.getState().future).toHaveLength(0);
  });

  it("replaces the present selection without creating a new undo step", () => {
    const store = useSelectionHistoryStore.getState();

    store.commitSelectionState({
      selection: { kind: "node", nodeId: "review" },
      terminalWorkDetail: null,
    });

    useSelectionHistoryStore.getState().replacePresent({
      selection: { kind: "node", nodeId: "implement" },
      terminalWorkDetail: null,
    });

    expect(useSelectionHistoryStore.getState().past).toHaveLength(1);
    expect(useSelectionHistoryStore.getState().present.selection).toEqual({
      kind: "node",
      nodeId: "implement",
    });

    useSelectionHistoryStore.getState().undo();

    expect(useSelectionHistoryStore.getState().present.selection).toBeNull();
  });
});
