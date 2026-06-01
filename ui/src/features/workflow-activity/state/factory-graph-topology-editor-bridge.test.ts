import { beforeEach, describe, expect, it } from "vitest";

import {
  readGraphDraftHasPendingChanges,
  useFactoryGraphTopologyEditorBridge,
} from "./factory-graph-topology-editor-bridge";

describe("useFactoryGraphTopologyEditorBridge graphDraftHasPendingChanges", () => {
  beforeEach(() => {
    useFactoryGraphTopologyEditorBridge.setState({
      graphDraftHasPendingChanges: false,
      handlers: null,
    });
  });

  it("registers graph draft pending state as false by default", () => {
    expect(readGraphDraftHasPendingChanges()).toBe(false);
    expect(
      useFactoryGraphTopologyEditorBridge.getState().graphDraftHasPendingChanges,
    ).toBe(false);
  });

  it("updates graph draft pending state when draft dirtiness changes", () => {
    useFactoryGraphTopologyEditorBridge
      .getState()
      .setGraphDraftHasPendingChanges(true);

    expect(readGraphDraftHasPendingChanges()).toBe(true);

    useFactoryGraphTopologyEditorBridge
      .getState()
      .setGraphDraftHasPendingChanges(false);

    expect(readGraphDraftHasPendingChanges()).toBe(false);
  });

  it("clears graph draft pending state when explicitly reset", () => {
    useFactoryGraphTopologyEditorBridge
      .getState()
      .setGraphDraftHasPendingChanges(true);

    useFactoryGraphTopologyEditorBridge
      .getState()
      .setGraphDraftHasPendingChanges(false);

    expect(readGraphDraftHasPendingChanges()).toBe(false);
  });

  it("allows current-selection code to read pending state without graph editor hooks", () => {
    useFactoryGraphTopologyEditorBridge
      .getState()
      .setGraphDraftHasPendingChanges(true);

    expect(readGraphDraftHasPendingChanges()).toBe(true);
  });
});
