import type { FactoryLayout } from "../factory-graph-layout-operations";
import {
  applyFactoryLayoutCommand,
  type FactoryLayoutCommand,
  factoryLayoutCommandReferencesDeletedNodeIds,
  invertFactoryLayoutCommand,
} from "./factory-graph-layout-commands";

export interface FactoryLayoutHistoryState {
  future: FactoryLayoutCommand[];
  past: FactoryLayoutCommand[];
}

export const FACTORY_LAYOUT_HISTORY_LIMIT = 100;

export function createFactoryLayoutHistoryState(): FactoryLayoutHistoryState {
  return {
    future: [],
    past: [],
  };
}

export function pushFactoryLayoutHistoryCommand(
  history: FactoryLayoutHistoryState,
  command: FactoryLayoutCommand,
): FactoryLayoutHistoryState {
  const past = [...history.past, command];
  if (past.length > FACTORY_LAYOUT_HISTORY_LIMIT) {
    past.shift();
  }

  return {
    future: [],
    past,
  };
}

export function canUndoFactoryLayoutHistory(
  history: FactoryLayoutHistoryState,
): boolean {
  return history.past.length > 0;
}

export function canRedoFactoryLayoutHistory(
  history: FactoryLayoutHistoryState,
): boolean {
  return history.future.length > 0;
}

export function undoFactoryLayoutHistory(
  history: FactoryLayoutHistoryState,
  layout: FactoryLayout,
): {
  command: FactoryLayoutCommand | null;
  history: FactoryLayoutHistoryState;
  layout: FactoryLayout;
} {
  if (history.past.length === 0) {
    return { command: null, history, layout };
  }

  const past = [...history.past];
  const command = past.pop();
  if (!command) {
    return { command: null, history, layout };
  }

  const inverse = invertFactoryLayoutCommand(command);
  return {
    command,
    history: {
      future: [command, ...history.future],
      past,
    },
    layout: applyFactoryLayoutCommand(layout, inverse),
  };
}

export function redoFactoryLayoutHistory(
  history: FactoryLayoutHistoryState,
  layout: FactoryLayout,
): {
  command: FactoryLayoutCommand | null;
  history: FactoryLayoutHistoryState;
  layout: FactoryLayout;
} {
  if (history.future.length === 0) {
    return { command: null, history, layout };
  }

  const future = [...history.future];
  const command = future.shift();
  if (!command) {
    return { command: null, history, layout };
  }

  return {
    command,
    history: {
      future,
      past: [...history.past, command],
    },
    layout: applyFactoryLayoutCommand(layout, command),
  };
}

export function pruneFactoryLayoutHistoryForNodeIds(
  history: FactoryLayoutHistoryState,
  activeNodeIds: ReadonlySet<string>,
): FactoryLayoutHistoryState {
  return {
    future: history.future.filter(
      (command) =>
        !factoryLayoutCommandReferencesDeletedNodeIds(command, activeNodeIds),
    ),
    past: history.past.filter(
      (command) =>
        !factoryLayoutCommandReferencesDeletedNodeIds(command, activeNodeIds),
    ),
  };
}

export function clearFactoryLayoutHistoryState(): FactoryLayoutHistoryState {
  return createFactoryLayoutHistoryState();
}
