import type { DashboardSnapshot } from "../../../api/dashboard/types";

type CurrentActivityFactoryDocumentStatus = "error" | "pending" | "success";

export interface CurrentActivityFactoryGraphSource {
  baseFactoryDocument?: DashboardSnapshot["factory"] | null;
  editableFactoryDocument?: DashboardSnapshot["factory"] | null;
  editableFactoryDocumentStatus?: CurrentActivityFactoryDocumentStatus;
  editorMode: boolean;
  latestFactoryDocument?: DashboardSnapshot["factory"] | null;
  pendingFactoryDefinition?: DashboardSnapshot["factory"] | null;
}

export function currentActivityCardPendingFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return source.pendingFactoryDefinition ?? null;
}

export function currentActivityCardCurrentFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null {
  return (
    source.pendingFactoryDefinition ??
    source.latestFactoryDocument ??
    source.baseFactoryDocument ??
    source.editableFactoryDocument ??
    null
  );
}

export function currentActivityCardFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null | undefined {
  if (!source.editorMode) {
    return null;
  }

  if (source.editableFactoryDocumentStatus !== "success") {
    return null;
  }

  return currentActivityCardCurrentFactoryDefinition(source);
}

export function currentActivityCardDisplayFactoryDefinition(
  source: CurrentActivityFactoryGraphSource,
): DashboardSnapshot["factory"] | null | undefined {
  return currentActivityCardFactoryDefinition(source);
}
