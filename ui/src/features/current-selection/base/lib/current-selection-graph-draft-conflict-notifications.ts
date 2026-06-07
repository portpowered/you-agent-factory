import { getCurrentSelectionGraphDraftConflictMessages } from "../messages/operational/current-selection-graph-draft-conflict";

export const CURRENT_SELECTION_GRAPH_DRAFT_CONFLICT_WARNING_KEY =
  "graph-draft-conflict-warning";

export type CurrentSelectionGraphDraftConflictNotification = {
  description: string;
  key: string;
  kind: "warning";
  title: string;
};

export function resolveCurrentSelectionGraphDraftConflictNotification({
  graphDraftHasPendingChanges,
  isTopologyAffectingSave,
  locale,
  saveSucceeded,
}: {
  graphDraftHasPendingChanges: boolean;
  isTopologyAffectingSave: boolean;
  locale?: string | null;
  saveSucceeded: boolean;
}): CurrentSelectionGraphDraftConflictNotification | null {
  if (
    !saveSucceeded ||
    !isTopologyAffectingSave ||
    !graphDraftHasPendingChanges
  ) {
    return null;
  }

  const messages = getCurrentSelectionGraphDraftConflictMessages(locale);

  return {
    description: messages.graphDraftConflictWarningDescription,
    key: CURRENT_SELECTION_GRAPH_DRAFT_CONFLICT_WARNING_KEY,
    kind: "warning",
    title: messages.graphDraftConflictWarningTitle,
  };
}
