import type { FactoryGraphConnectionAnchorContext } from "./factory-graph-editor-connections";
import { getFactoryGraphConnectionAnchors } from "./factory-graph-editor-connections";
import { PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS } from "./factory-graph-progress-outcome-connection-anchors";

/** True when progress-outcome handle validation and selection messages may surface for an anchor. */
export function workstationRendersProgressOutcomeHandleValidation(
  context: FactoryGraphConnectionAnchorContext,
  anchorId: string,
): boolean {
  if (!PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS.has(anchorId)) {
    return true;
  }

  return getFactoryGraphConnectionAnchors("workstation", context).some(
    (anchor) => anchor.id === anchorId,
  );
}

/** True when Continue/Reject connect anchors are on the rendered workstation rail. */
export function workstationRendersProgressOutcomeZAxisHintAnchors(
  context: FactoryGraphConnectionAnchorContext,
): boolean {
  const renderedIds = new Set(
    getFactoryGraphConnectionAnchors("workstation", context).map(
      (anchor) => anchor.id,
    ),
  );

  for (const anchorId of PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS) {
    if (!renderedIds.has(anchorId)) {
      return false;
    }
  }

  return true;
}
