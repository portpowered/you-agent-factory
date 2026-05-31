export function workStateGraphNodeId(
  workTypeName: string,
  stateName: string,
): string {
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `work-state:${workTypeName}:${stateName}`;
}
