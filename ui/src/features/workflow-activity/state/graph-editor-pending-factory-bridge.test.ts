import { beforeEach, describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { useGraphEditorPendingFactoryBridge } from "./graph-editor-pending-factory-bridge";

const pendingFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  supportingFiles: {
    bundledFiles: [
      {
        content: { encoding: "utf-8", inline: "# Draft\n" },
        targetPath: "factory/docs/draft.md",
        type: "DOC",
      },
    ],
  },
  workTypes: [],
};

describe("useGraphEditorPendingFactoryBridge", () => {
  beforeEach(() => {
    useGraphEditorPendingFactoryBridge.setState({
      pendingFactoryDefinition: null,
    });
  });

  it("starts with no pending factory definition", () => {
    expect(
      useGraphEditorPendingFactoryBridge.getState().pendingFactoryDefinition,
    ).toBeNull();
  });

  it("publishes and clears the graph-editor pending factory definition", () => {
    useGraphEditorPendingFactoryBridge
      .getState()
      .setPendingFactoryDefinition(pendingFactoryDefinition);

    expect(
      useGraphEditorPendingFactoryBridge.getState().pendingFactoryDefinition,
    ).toEqual(pendingFactoryDefinition);

    useGraphEditorPendingFactoryBridge
      .getState()
      .setPendingFactoryDefinition(null);

    expect(
      useGraphEditorPendingFactoryBridge.getState().pendingFactoryDefinition,
    ).toBeNull();
  });
});
