import type { FactoryGraphConnectionAnchor } from "./factory-graph-editor-connections";

export const PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS = new Set([
  "workstation-on-continue-source",
  "workstation-on-rejection-source",
]);

const WORKSTATION_PROGRESS_OUTCOME_SOURCE_ANCHORS: FactoryGraphConnectionAnchor[] =
  [
    {
      description: "",
      edgeKind: "workstation-on-continue",
      id: "workstation-on-continue-source",
      label: "",
      role: "source",
      side: "right",
    },
    {
      description: "",
      edgeKind: "workstation-on-rejection",
      id: "workstation-on-rejection-source",
      label: "",
      role: "source",
      side: "right",
    },
  ];

export function mergeAuthoredProgressOutcomeConnectionAnchors<
  T extends FactoryGraphConnectionAnchor,
>(
  anchors: readonly T[],
  requiredSourceHandleIds: ReadonlySet<string> | undefined,
): T[] {
  if (!requiredSourceHandleIds?.size) {
    return [...anchors];
  }

  const presentIds = new Set(anchors.map((anchor) => anchor.id));
  const extras = WORKSTATION_PROGRESS_OUTCOME_SOURCE_ANCHORS.filter(
    (anchor) =>
      PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS.has(anchor.id) &&
      requiredSourceHandleIds.has(anchor.id) &&
      !presentIds.has(anchor.id),
  ) as T[];

  return extras.length === 0 ? [...anchors] : [...anchors, ...extras];
}
