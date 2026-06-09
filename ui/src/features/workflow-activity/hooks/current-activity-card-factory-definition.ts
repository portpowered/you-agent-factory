import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  preserveExistingBundledFilesWhenAbsent,
  preserveExistingLayoutWhenAbsent,
} from "../../../api/factory-definition";

type CurrentActivityFactoryDefinition = NonNullable<
  DashboardSnapshot["factory"]
>;
type CurrentActivityFactoryDocumentStatus = "error" | "pending" | "success";

export interface CurrentActivityFactoryGraphSource {
  draftState: {
    baseDocument?: DashboardSnapshot["factory"] | null;
    latestDocument?: DashboardSnapshot["factory"] | null;
    pendingFactoryDefinition?: DashboardSnapshot["factory"] | null;
  };
  editableFactoryDocument?: DashboardSnapshot["factory"] | null;
  editableFactoryDocumentStatus?: CurrentActivityFactoryDocumentStatus;
  editorMode: boolean;
}

function observeModeSavedFactoryDocument(
  source: CurrentActivityFactoryGraphSource,
) {
  return (
    source.editableFactoryDocument ??
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
  incoming: CurrentActivityFactoryDefinition;
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
): DashboardSnapshot["factory"] | undefined {
  const document = observeModeSavedFactoryDocument(source);
  const eventComputedFactory = snapshot.factory ?? document;
  if (!eventComputedFactory) {
    return undefined;
  }

  return preserveObserverModeFactoryMetadata({
    documentFactory: document,
    incoming: eventComputedFactory,
    snapshotFactory: snapshot.factory,
  });
}

function editorModeFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
) {
  return currentActivityCardCurrentFactoryDefinition(source) ?? undefined;
}

export function currentActivityCardFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
): DashboardSnapshot["factory"] | null | undefined {
  if (!source.editorMode) {
    const document = observeModeSavedFactoryDocument(source);
    if (!document) {
      return (
        observeModeFactoryWithBundledDocs(snapshot.factory, document) ?? null
      );
    }

    return observeModeFactoryDefinition(source, snapshot);
  }

  if (source.editableFactoryDocumentStatus !== "success") {
    return null;
  }

  return editorModeFactoryDefinition(source) ?? null;
}

export function currentActivityCardDisplayFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
): DashboardSnapshot["factory"] | null | undefined {
  const document = observeModeSavedFactoryDocument(source);
  const resolvedFactory = currentActivityCardFactoryDefinition(
    source,
    snapshot,
  );

  if (source.editorMode) {
    return resolvedFactory ?? undefined;
  }

  return observeModeFactoryWithBundledDocs(resolvedFactory, document);
}
