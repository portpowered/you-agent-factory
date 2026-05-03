import { create } from "zustand";

import type { DashboardSelection } from "./dashboardSelection";
import type { TerminalWorkDetail } from "../types";

const SELECTION_HISTORY_LIMIT = 50;

export interface SelectionHistoryEntry {
  selection: DashboardSelection | null;
  terminalWorkDetail: TerminalWorkDetail | null;
}

interface SelectionHistoryStoreState {
  future: SelectionHistoryEntry[];
  past: SelectionHistoryEntry[];
  present: SelectionHistoryEntry;
  clear: () => void;
  commitSelectionState: (entry: SelectionHistoryEntry) => void;
  redo: () => void;
  replacePresent: (entry: SelectionHistoryEntry) => void;
  undo: () => void;
}

const EMPTY_SELECTION_HISTORY_ENTRY: SelectionHistoryEntry = {
  selection: null,
  terminalWorkDetail: null,
};

function selectionHistorySelectionKey(selection: DashboardSelection | null): string {
  if (!selection) {
    return "none";
  }

  switch (selection.kind) {
    case "node":
      return `node:${selection.nodeId}`;
    case "state-node":
      return `state:${selection.placeId}`;
    case "work-item":
      return `work:${selection.nodeId}:${selection.dispatchId ?? ""}:${selection.workItem.work_id}`;
    case "workstation-request":
      return `request:${selection.nodeId}:${selection.dispatchId}`;
  }
}

function selectionHistoryTerminalDetailKey(
  terminalWorkDetail: TerminalWorkDetail | null,
): string {
  if (!terminalWorkDetail) {
    return "none";
  }

  return [
    terminalWorkDetail.status,
    terminalWorkDetail.traceWorkID,
    terminalWorkDetail.label,
    terminalWorkDetail.failureReason ?? "",
    terminalWorkDetail.failureMessage ?? "",
  ].join(":");
}

function sameSelectionHistoryEntry(
  left: SelectionHistoryEntry,
  right: SelectionHistoryEntry,
): boolean {
  return (
    selectionHistorySelectionKey(left.selection) ===
      selectionHistorySelectionKey(right.selection) &&
    selectionHistoryTerminalDetailKey(left.terminalWorkDetail) ===
      selectionHistoryTerminalDetailKey(right.terminalWorkDetail)
  );
}

function boundedHistory(
  past: SelectionHistoryEntry[],
  nextEntry: SelectionHistoryEntry,
): SelectionHistoryEntry[] {
  return [...past, nextEntry].slice(-SELECTION_HISTORY_LIMIT);
}

export const useSelectionHistoryStore = create<SelectionHistoryStoreState>()((set) => ({
  future: [],
  past: [],
  present: EMPTY_SELECTION_HISTORY_ENTRY,
  clear: () => {
    set((state) => {
      if (
        state.past.length === 0 &&
        state.future.length === 0 &&
        sameSelectionHistoryEntry(state.present, EMPTY_SELECTION_HISTORY_ENTRY)
      ) {
        return state;
      }

      return {
        future: [],
        past: [],
        present: EMPTY_SELECTION_HISTORY_ENTRY,
      };
    });
  },
  commitSelectionState: (entry) => {
    set((state) => {
      if (sameSelectionHistoryEntry(state.present, entry)) {
        return state;
      }

      return {
        future: [],
        past: boundedHistory(state.past, state.present),
        present: entry,
      };
    });
  },
  redo: () => {
    set((state) => {
      const [nextPresent, ...nextFuture] = state.future;
      if (!nextPresent) {
        return state;
      }

      return {
        future: nextFuture,
        past: boundedHistory(state.past, state.present),
        present: nextPresent,
      };
    });
  },
  replacePresent: (entry) => {
    set((state) => {
      if (sameSelectionHistoryEntry(state.present, entry)) {
        return state;
      }

      return {
        ...state,
        present: entry,
      };
    });
  },
  undo: () => {
    set((state) => {
      const nextPast = state.past.slice(0, -1);
      const nextPresent = state.past[state.past.length - 1];
      if (!nextPresent) {
        return state;
      }

      return {
        future: [state.present, ...state.future],
        past: nextPast,
        present: nextPresent,
      };
    });
  },
}));

export function resetSelectionHistoryStore(): void {
  useSelectionHistoryStore.setState({
    future: [],
    past: [],
    present: EMPTY_SELECTION_HISTORY_ENTRY,
  });
}

