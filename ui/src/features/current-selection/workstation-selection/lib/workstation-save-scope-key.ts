export interface ParsedWorkstationSaveScopeKey {
  nodeId: string;
  transitionId: string;
  workstationName: string;
}

export function buildWorkstationSaveScopeKey({
  nodeId,
  transitionId,
  workstationName,
}: {
  nodeId: string;
  transitionId: string;
  workstationName: string;
}): string {
  return `${nodeId}:${transitionId}:${workstationName}`;
}

export function parseWorkstationSaveScopeKey(
  scopeKey: string,
): ParsedWorkstationSaveScopeKey | null {
  const firstColon = scopeKey.indexOf(":");
  if (firstColon === -1) {
    return null;
  }
  const secondColon = scopeKey.indexOf(":", firstColon + 1);
  if (secondColon === -1) {
    return null;
  }

  return {
    nodeId: scopeKey.slice(0, firstColon),
    transitionId: scopeKey.slice(firstColon + 1, secondColon),
    workstationName: scopeKey.slice(secondColon + 1),
  };
}
