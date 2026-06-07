import { describe, expect, it } from "vitest";

import {
  hasPortableFactoryDocumentChanges,
  resolveFactoryGraphEditorDirtyState,
  resolveFactoryGraphSaveSummaryKind,
} from "./factory-graph-editor-dirty-state";

describe("factory graph editor dirty state", () => {
  it("treats layout and topology dirty flags independently", () => {
    const dirty = resolveFactoryGraphEditorDirtyState({
      hasLayoutChanges: true,
      hasPreferenceChanges: false,
      hasTopologyChanges: false,
    });

    expect(dirty.layoutDirty).toBe(true);
    expect(dirty.topologyDirty).toBe(false);
    expect(hasPortableFactoryDocumentChanges(dirty)).toBe(true);
    expect(resolveFactoryGraphSaveSummaryKind(dirty)).toBe("layout-only");
  });

  it("keeps preferences dirty separate from portable document changes", () => {
    const dirty = resolveFactoryGraphEditorDirtyState({
      hasLayoutChanges: false,
      hasPreferenceChanges: true,
      hasTopologyChanges: false,
    });

    expect(hasPortableFactoryDocumentChanges(dirty)).toBe(false);
    expect(resolveFactoryGraphSaveSummaryKind(dirty)).toBe("preferences-only");
  });
});
