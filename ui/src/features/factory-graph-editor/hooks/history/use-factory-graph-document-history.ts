import { useCallback, useRef, useState } from "react";

import type { FactoryGraphDraft } from "../../lib/draft/factory-graph-draft-types";
import {
  canRedoFactoryGraphDocumentHistory,
  canUndoFactoryGraphDocumentHistory,
  createFactoryGraphDocumentHistoryState,
  type FactoryGraphDocumentHistoryState,
  type FactoryGraphDocumentSnapshot,
  factoryGraphDocumentSnapshotsEqual,
  recordFactoryGraphDocumentTransaction,
  redoFactoryGraphDocumentHistory,
  resetFactoryGraphDocumentHistory,
  undoFactoryGraphDocumentHistory,
} from "../../lib/editor-runtime/document-history/factory-graph-document-history";
import type { FactoryLayout } from "../../lib/layout/factory-graph-layout-operations";

export function useFactoryGraphDocumentHistory(
  initialSnapshot: FactoryGraphDocumentSnapshot,
) {
  const [history, setHistory] = useState<FactoryGraphDocumentHistoryState>(() =>
    createFactoryGraphDocumentHistoryState(initialSnapshot),
  );
  const historyRef = useRef(history);

  const publish = useCallback(
    (nextHistory: FactoryGraphDocumentHistoryState) => {
      historyRef.current = nextHistory;
      setHistory(nextHistory);
    },
    [],
  );
  const record = useCallback(
    (snapshot: FactoryGraphDocumentSnapshot) => {
      const nextHistory = recordFactoryGraphDocumentTransaction(
        historyRef.current,
        snapshot,
      );
      if (nextHistory !== historyRef.current) {
        publish(nextHistory);
      }
    },
    [publish],
  );
  const recordDraft = useCallback(
    (draft: FactoryGraphDraft) => {
      record({
        draft,
        layout: historyRef.current.present.layout,
      });
    },
    [record],
  );
  const recordLayout = useCallback(
    (layout: FactoryLayout) => {
      record({
        draft: historyRef.current.present.draft,
        layout,
      });
    },
    [record],
  );
  const undo = useCallback(() => {
    const result = undoFactoryGraphDocumentHistory(historyRef.current);
    if (result.snapshot) {
      publish(result.history);
    }
    return result.snapshot;
  }, [publish]);
  const redo = useCallback(() => {
    const result = redoFactoryGraphDocumentHistory(historyRef.current);
    if (result.snapshot) {
      publish(result.history);
    }
    return result.snapshot;
  }, [publish]);
  const reset = useCallback(
    (snapshot: FactoryGraphDocumentSnapshot) => {
      publish(resetFactoryGraphDocumentHistory(snapshot));
    },
    [publish],
  );
  const reconcile = useCallback(
    (snapshot: FactoryGraphDocumentSnapshot) => {
      if (
        factoryGraphDocumentSnapshotsEqual(historyRef.current.present, snapshot)
      ) {
        return;
      }
      reset(snapshot);
    },
    [reset],
  );

  return {
    canRedo: canRedoFactoryGraphDocumentHistory(history),
    canUndo: canUndoFactoryGraphDocumentHistory(history),
    history,
    present: history.present,
    record,
    recordDraft,
    recordLayout,
    redo,
    reconcile,
    reset,
    undo,
  };
}
