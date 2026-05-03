import type { DashboardWorkstationNode } from "../../api/dashboard/types";
import type { GraphSemanticIconKind } from "./graph-semantic-icon";
import { EXHAUSTION_WORKSTATION_KIND, isExhaustionWorkstation } from "./workstation-semantics";

export const STANDARD_WORKSTATION_KIND = "standard";
export const REPEATER_WORKSTATION_KIND = "repeater";
export const CRON_WORKSTATION_KIND = "cron";

export const SUPPORTED_WORKSTATION_ICON_KINDS = [
  STANDARD_WORKSTATION_KIND,
  REPEATER_WORKSTATION_KIND,
  CRON_WORKSTATION_KIND,
] as const;

export type SupportedWorkstationIconKind = (typeof SUPPORTED_WORKSTATION_ICON_KINDS)[number];
export type WorkstationSemanticKind =
  | SupportedWorkstationIconKind
  | typeof EXHAUSTION_WORKSTATION_KIND;

export interface WorkstationIconMetadata {
  className: string;
  iconKind: GraphSemanticIconKind;
  label: string;
  semanticKind: WorkstationSemanticKind;
}

const WORKSTATION_ICON_METADATA_BY_KIND = {
  [CRON_WORKSTATION_KIND]: {
    className: "text-af-success-ink/76",
    iconKind: "cron",
    label: "Cron workstation",
    semanticKind: CRON_WORKSTATION_KIND,
  },
  [EXHAUSTION_WORKSTATION_KIND]: {
    className: "text-af-danger-ink/76",
    iconKind: "exhaustion",
    label: "Exhaustion rule",
    semanticKind: EXHAUSTION_WORKSTATION_KIND,
  },
  [REPEATER_WORKSTATION_KIND]: {
    className: "text-af-info/78",
    iconKind: "repeater",
    label: "Repeater workstation",
    semanticKind: REPEATER_WORKSTATION_KIND,
  },
  [STANDARD_WORKSTATION_KIND]: {
    className: "text-af-ink/62",
    iconKind: "workstation",
    label: "Standard workstation",
    semanticKind: STANDARD_WORKSTATION_KIND,
  },
} satisfies Record<WorkstationSemanticKind, WorkstationIconMetadata>;

export const SUPPORTED_WORKSTATION_ICON_METADATA = SUPPORTED_WORKSTATION_ICON_KINDS.map(
  (kind) => WORKSTATION_ICON_METADATA_BY_KIND[kind],
);
export const EXHAUSTION_WORKSTATION_ICON_METADATA =
  WORKSTATION_ICON_METADATA_BY_KIND[EXHAUSTION_WORKSTATION_KIND];

export function workstationSemanticKind(
  workstation: DashboardWorkstationNode,
): WorkstationSemanticKind {
  if (isExhaustionWorkstation(workstation)) {
    return EXHAUSTION_WORKSTATION_KIND;
  }
  if (workstation.workstation_kind === REPEATER_WORKSTATION_KIND) {
    return REPEATER_WORKSTATION_KIND;
  }
  if (workstation.workstation_kind === CRON_WORKSTATION_KIND) {
    return CRON_WORKSTATION_KIND;
  }
  return STANDARD_WORKSTATION_KIND;
}

export function workstationIconMetadata(
  workstation: DashboardWorkstationNode,
): WorkstationIconMetadata {
  return WORKSTATION_ICON_METADATA_BY_KIND[workstationSemanticKind(workstation)];
}

