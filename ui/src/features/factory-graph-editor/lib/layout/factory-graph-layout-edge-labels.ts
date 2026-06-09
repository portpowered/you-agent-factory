export function describeFactoryGraphLayoutEdgeId(edgeId: string): {
  kind: string;
  sourceId: string;
  targetId: string;
} {
  const separatorIndex = edgeId.indexOf(":");
  const arrowIndex = edgeId.indexOf("->");
  if (separatorIndex < 0 || arrowIndex < 0) {
    return {
      kind: edgeId,
      sourceId: edgeId,
      targetId: edgeId,
    };
  }

  return {
    kind: edgeId.slice(0, separatorIndex),
    sourceId: edgeId.slice(separatorIndex + 1, arrowIndex),
    targetId: edgeId.slice(arrowIndex + 2),
  };
}
