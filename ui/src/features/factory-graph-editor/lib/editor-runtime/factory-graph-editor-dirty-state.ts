export interface FactoryGraphEditorDirtyState {
  layoutDirty: boolean;
  preferencesDirty: boolean;
  topologyDirty: boolean;
}

export type FactoryGraphSaveSummaryKind =
  | "layout-only"
  | "mixed"
  | "none"
  | "preferences-only"
  | "topology-only";

export function resolveFactoryGraphEditorDirtyState(input: {
  hasLayoutChanges: boolean;
  hasPreferenceChanges: boolean;
  hasTopologyChanges: boolean;
}): FactoryGraphEditorDirtyState {
  return {
    layoutDirty: input.hasLayoutChanges,
    preferencesDirty: input.hasPreferenceChanges,
    topologyDirty: input.hasTopologyChanges,
  };
}

export function hasPortableFactoryDocumentChanges(
  dirty: FactoryGraphEditorDirtyState,
): boolean {
  return dirty.layoutDirty || dirty.topologyDirty;
}

export function hasAnyFactoryGraphEditorChanges(
  dirty: FactoryGraphEditorDirtyState,
): boolean {
  return hasPortableFactoryDocumentChanges(dirty) || dirty.preferencesDirty;
}

export function resolveFactoryGraphSaveSummaryKind(
  dirty: FactoryGraphEditorDirtyState,
): FactoryGraphSaveSummaryKind {
  if (!dirty.layoutDirty && !dirty.topologyDirty && !dirty.preferencesDirty) {
    return "none";
  }

  if (dirty.preferencesDirty && !dirty.layoutDirty && !dirty.topologyDirty) {
    return "preferences-only";
  }

  if (dirty.layoutDirty && dirty.topologyDirty) {
    return "mixed";
  }

  if (dirty.layoutDirty) {
    return "layout-only";
  }

  if (dirty.topologyDirty) {
    return "topology-only";
  }

  return "none";
}
