import type { FactoryGraphDraft } from "../draft/factory-graph-draft-types";
import type { FactoryLayout } from "../layout/factory-graph-layout-operations";

export interface FactoryGraphDocumentSnapshot {
  draft: FactoryGraphDraft;
  layout: FactoryLayout;
}

export interface FactoryGraphDocumentTransaction {
  after: FactoryGraphDocumentSnapshot;
  before: FactoryGraphDocumentSnapshot;
}

export interface FactoryGraphDocumentHistoryState {
  future: FactoryGraphDocumentTransaction[];
  past: FactoryGraphDocumentTransaction[];
  present: FactoryGraphDocumentSnapshot;
}

export const FACTORY_GRAPH_DOCUMENT_HISTORY_LIMIT = 100;

export function createFactoryGraphDocumentHistoryState(
  snapshot: FactoryGraphDocumentSnapshot,
): FactoryGraphDocumentHistoryState {
  return {
    future: [],
    past: [],
    present: cloneSnapshot(snapshot),
  };
}

export function recordFactoryGraphDocumentTransaction(
  history: FactoryGraphDocumentHistoryState,
  snapshot: FactoryGraphDocumentSnapshot,
): FactoryGraphDocumentHistoryState {
  const after = cloneSnapshot(snapshot);
  if (factoryGraphDocumentSnapshotsEqual(history.present, after)) {
    return history;
  }

  const past = [
    ...history.past,
    {
      after,
      before: cloneSnapshot(history.present),
    },
  ];
  if (past.length > FACTORY_GRAPH_DOCUMENT_HISTORY_LIMIT) {
    past.shift();
  }

  return {
    future: [],
    past,
    present: after,
  };
}

export function canUndoFactoryGraphDocumentHistory(
  history: FactoryGraphDocumentHistoryState,
): boolean {
  return history.past.length > 0;
}

export function canRedoFactoryGraphDocumentHistory(
  history: FactoryGraphDocumentHistoryState,
): boolean {
  return history.future.length > 0;
}

export function undoFactoryGraphDocumentHistory(
  history: FactoryGraphDocumentHistoryState,
): {
  history: FactoryGraphDocumentHistoryState;
  snapshot: FactoryGraphDocumentSnapshot | null;
} {
  const transaction = history.past.at(-1);
  if (!transaction) {
    return { history, snapshot: null };
  }

  return {
    history: {
      future: [transaction, ...history.future],
      past: history.past.slice(0, -1),
      present: cloneSnapshot(transaction.before),
    },
    snapshot: cloneSnapshot(transaction.before),
  };
}

export function redoFactoryGraphDocumentHistory(
  history: FactoryGraphDocumentHistoryState,
): {
  history: FactoryGraphDocumentHistoryState;
  snapshot: FactoryGraphDocumentSnapshot | null;
} {
  const transaction = history.future[0];
  if (!transaction) {
    return { history, snapshot: null };
  }

  return {
    history: {
      future: history.future.slice(1),
      past: [...history.past, transaction],
      present: cloneSnapshot(transaction.after),
    },
    snapshot: cloneSnapshot(transaction.after),
  };
}

export function resetFactoryGraphDocumentHistory(
  snapshot: FactoryGraphDocumentSnapshot,
): FactoryGraphDocumentHistoryState {
  return createFactoryGraphDocumentHistoryState(snapshot);
}

export function factoryGraphDocumentSnapshotsEqual(
  left: FactoryGraphDocumentSnapshot,
  right: FactoryGraphDocumentSnapshot,
): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

function cloneSnapshot(
  snapshot: FactoryGraphDocumentSnapshot,
): FactoryGraphDocumentSnapshot {
  return structuredClone(snapshot);
}
