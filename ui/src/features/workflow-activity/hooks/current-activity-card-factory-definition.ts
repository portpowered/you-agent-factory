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
  baseFactoryDocument?: DashboardSnapshot["factory"] | null;
  editableFactoryDocument?: DashboardSnapshot["factory"] | null;
  editableFactoryDocumentStatus?: CurrentActivityFactoryDocumentStatus;
  editorMode: boolean;
  latestFactoryDocument?: DashboardSnapshot["factory"] | null;
  pendingFactoryDefinition?: DashboardSnapshot["factory"] | null;
}

function observeModeSavedFactoryDocument(
  source: CurrentActivityFactoryGraphSource,
) {
  return (
    source.editableFactoryDocument ??
    source.latestFactoryDocument ??
    source.baseFactoryDocument ??
    undefined
  );
}

export function currentActivityCardPendingFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return source.pendingFactoryDefinition ?? null;
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

export function currentActivityCardCurrentFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return (
    source.pendingFactoryDefinition ??
    source.latestFactoryDocument ??
    source.baseFactoryDocument ??
    null
  );
}

export function currentActivityCardFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
): DashboardSnapshot["factory"] | null | undefined {
  if (!source.editorMode) {
    return observeModeFactoryDefinition(source, snapshot) ?? null;
  }

  if (source.editableFactoryDocumentStatus !== "success") {
    return null;
  }

  return currentActivityCardCurrentFactoryDefinition(source);
}

export function currentActivityCardDisplayFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
): DashboardSnapshot["factory"] | null | undefined {
  const document = observeModeSavedFactoryDocument(source);
  const resolvedFactory = currentActivityCardFactoryDefinition(source, snapshot);

  if (source.editorMode) {
    return resolvedFactory ?? undefined;
  }

  if (!document) {
    return resolvedFactory ?? undefined;
  }

  return preserveObserverModeFactoryMetadata({
    documentFactory: document,
    incoming: resolvedFactory ?? document,
    snapshotFactory: snapshot.factory,
  });
}
