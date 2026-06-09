import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  preserveExistingBundledFilesWhenAbsent,
  preserveExistingLayoutWhenAbsent,
} from "../../../api/factory-definition";
import type { FactoryTimelineMode } from "../../timeline/state/factoryTimelineStore";
import { resolveObserveModeFactoryDefinition } from "./observe-mode-factory-definition";

export interface CurrentActivityFactoryGraphSource {
  draftState: {
    baseDocument?: DashboardSnapshot["factory"] | null;
    latestDocument?: DashboardSnapshot["factory"] | null;
    pendingFactoryDefinition?: DashboardSnapshot["factory"] | null;
  };
  editableDefinitionQuery?: {
    data?: DashboardSnapshot["factory"] | null;
    status?: "error" | "pending" | "success";
  } | null;
  editorMode: boolean;
}

function observeModeSavedFactoryDocument(
  source: CurrentActivityFactoryGraphSource,
) {
  return (
    source.editableDefinitionQuery?.data ??
    source.draftState.latestDocument ??
    source.draftState.baseDocument ??
    undefined
  );
}

export function currentActivityCardSavedFactoryDocument(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return observeModeSavedFactoryDocument(source) ?? null;
}

export function currentActivityCardPendingFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return source.draftState.pendingFactoryDefinition ?? null;
}

export function currentActivityCardBaseFactoryDocument(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return (
    source.draftState.baseDocument ??
    observeModeSavedFactoryDocument(source) ??
    null
  );
}

export function currentActivityCardCurrentFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return (
    source.draftState.pendingFactoryDefinition ??
    source.draftState.latestDocument ??
    source.draftState.baseDocument ??
    null
  );
}

function observeModeFactoryWithBundledDocs(
  factory: DashboardSnapshot["factory"] | null | undefined,
  document: DashboardSnapshot["factory"] | null | undefined,
) {
  if (!document) {
    return factory ?? undefined;
  }

  return preserveObserverModeFactoryMetadata({
    documentFactory: document,
    incoming: factory ?? document,
    snapshotFactory: factory,
  });
}

function preserveObserverModeFactoryMetadata({
  documentFactory,
  incoming,
  snapshotFactory,
}: {
  documentFactory: DashboardSnapshot["factory"] | null | undefined;
  incoming: DashboardSnapshot["factory"];
  snapshotFactory: DashboardSnapshot["factory"] | null | undefined;
}) {
  return preserveExistingBundledFilesWhenAbsent(
    preserveExistingLayoutWhenAbsent(incoming, snapshotFactory),
    documentFactory,
  );
}

function observeModeFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
  timelineMode: FactoryTimelineMode,
): DashboardSnapshot["factory"] | undefined {
  const document = observeModeSavedFactoryDocument(source);
  if (!document) {
    return snapshot.factory;
  }

  const resolvedFactory = resolveObserveModeFactoryDefinition({
    document,
    snapshotFactory: snapshot.factory,
    timelineMode,
  });

  return preserveObserverModeFactoryMetadata({
    documentFactory: document,
    incoming: resolvedFactory,
    snapshotFactory: snapshot.factory,
  });
}

function editorModeFactoryDefinition(source: CurrentActivityFactoryGraphSource) {
  return currentActivityCardCurrentFactoryDefinition(source) ?? undefined;
}

export function currentActivityCardFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
  timelineMode: FactoryTimelineMode,
): DashboardSnapshot["factory"] | null | undefined {
  if (!source.editorMode) {
    const document = observeModeSavedFactoryDocument(source);
    if (!document) {
      return observeModeFactoryWithBundledDocs(snapshot.factory, document) ?? null;
    }

    return observeModeFactoryDefinition(source, snapshot, timelineMode);
  }

  if (source.editableDefinitionQuery?.status !== "success") {
    return null;
  }

  return editorModeFactoryDefinition(source) ?? null;
}

export function currentActivityCardDisplayFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
  timelineMode: FactoryTimelineMode,
): DashboardSnapshot["factory"] | null | undefined {
  const document = observeModeSavedFactoryDocument(source);
  const resolvedFactory = currentActivityCardFactoryDefinition(
    source,
    snapshot,
    timelineMode,
  );

  if (source.editorMode) {
    return resolvedFactory ?? undefined;
  }

  return observeModeFactoryWithBundledDocs(resolvedFactory, document);
}
