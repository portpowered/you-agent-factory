import { useRef } from "react";

import {
  guardsDraftEqual,
  type WorkstationLevelGuard,
} from "../../../../current-factory-definition/lib/workstation-guards";

export function findRemovedWorkstationGuardIndex(
  previousGuards: WorkstationLevelGuard[],
  nextGuards: WorkstationLevelGuard[],
): number {
  if (nextGuards.length !== previousGuards.length - 1) {
    return -1;
  }

  for (
    let removedIndex = 0;
    removedIndex < previousGuards.length;
    removedIndex += 1
  ) {
    const candidate = [
      ...previousGuards.slice(0, removedIndex),
      ...previousGuards.slice(removedIndex + 1),
    ];
    if (guardsDraftEqual(candidate, nextGuards)) {
      return removedIndex;
    }
  }

  return previousGuards.length - 1;
}

export function useStableWorkstationGuardRowKeys(
  guards: WorkstationLevelGuard[],
): string[] {
  const stateRef = useRef<StableWorkstationGuardRowKeyState>({
    nextKey: 0,
    previousGuards: guards,
    rowKeys: [],
  });
  stateRef.current = resolveStableWorkstationGuardRowKeys(
    stateRef.current,
    guards,
  );
  return stateRef.current.rowKeys;
}

export interface StableWorkstationGuardRowKeyState {
  nextKey: number;
  previousGuards: WorkstationLevelGuard[];
  rowKeys: string[];
}

export function resolveStableWorkstationGuardRowKeys(
  previousState: StableWorkstationGuardRowKeyState,
  guards: WorkstationLevelGuard[],
): StableWorkstationGuardRowKeyState {
  const rowKeys = [...previousState.rowKeys];
  let nextKey = previousState.nextKey;
  const previousGuards = previousState.previousGuards;

  if (guards.length > previousGuards.length) {
    const addedCount = guards.length - previousGuards.length;
    for (let index = 0; index < addedCount; index += 1) {
      nextKey += 1;
      rowKeys.push(`workstation-guard-row-${nextKey}`);
    }
  } else if (guards.length < previousGuards.length) {
    const removedIndex = findRemovedWorkstationGuardIndex(
      previousGuards,
      guards,
    );
    if (removedIndex >= 0) {
      rowKeys.splice(removedIndex, 1);
    } else {
      rowKeys.splice(guards.length);
    }
  }

  while (rowKeys.length < guards.length) {
    nextKey += 1;
    rowKeys.push(`workstation-guard-row-${nextKey}`);
  }

  return { nextKey, previousGuards: guards, rowKeys };
}
