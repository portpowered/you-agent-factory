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
  const rowKeysRef = useRef<string[]>([]);
  const nextKeyRef = useRef(0);
  const previousGuardsRef = useRef(guards);

  const previousGuards = previousGuardsRef.current;

  if (guards.length > previousGuards.length) {
    const addedCount = guards.length - previousGuards.length;
    for (let index = 0; index < addedCount; index += 1) {
      nextKeyRef.current += 1;
      rowKeysRef.current.push(`workstation-guard-row-${nextKeyRef.current}`);
    }
  } else if (guards.length < previousGuards.length) {
    const removedIndex = findRemovedWorkstationGuardIndex(
      previousGuards,
      guards,
    );
    if (removedIndex >= 0) {
      rowKeysRef.current.splice(removedIndex, 1);
    } else {
      rowKeysRef.current = rowKeysRef.current.slice(0, guards.length);
    }
  }

  while (rowKeysRef.current.length < guards.length) {
    nextKeyRef.current += 1;
    rowKeysRef.current.push(`workstation-guard-row-${nextKeyRef.current}`);
  }

  previousGuardsRef.current = guards;
  return rowKeysRef.current;
}
