import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import type { components } from "../../../api/generated/openapi";
import type { GraphSemanticIconKind } from "../components/graph-semantic-icon";
import { getActivityGraphMessages } from "../messages/activity-graph";
import {
  EXHAUSTION_WORKSTATION_KIND,
  isExhaustionWorkstation,
} from "./workstation-semantics";

export type ApiWorkstationKind = components["schemas"]["WorkstationKind"];

export const STANDARD_WORKSTATION_KIND =
  "STANDARD" satisfies ApiWorkstationKind;
export const REPEATER_WORKSTATION_KIND =
  "REPEATER" satisfies ApiWorkstationKind;
export const CRON_WORKSTATION_KIND = "CRON" satisfies ApiWorkstationKind;
export const POLLER_WORKSTATION_KIND = "POLLER" satisfies ApiWorkstationKind;

export const SUPPORTED_WORKSTATION_ICON_KINDS = [
  STANDARD_WORKSTATION_KIND,
  REPEATER_WORKSTATION_KIND,
  CRON_WORKSTATION_KIND,
  POLLER_WORKSTATION_KIND,
] as const;

export type SupportedWorkstationIconKind =
  (typeof SUPPORTED_WORKSTATION_ICON_KINDS)[number];
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
    className: "text-af-success",
    iconKind: "cron",
    label: getActivityGraphMessages().workstationIconLabel(
      CRON_WORKSTATION_KIND,
    ),
    semanticKind: CRON_WORKSTATION_KIND,
  },
  [POLLER_WORKSTATION_KIND]: {
    className: "text-af-accent",
    iconKind: "poller",
    label: getActivityGraphMessages().workstationIconLabel(
      POLLER_WORKSTATION_KIND,
    ),
    semanticKind: POLLER_WORKSTATION_KIND,
  },
  [EXHAUSTION_WORKSTATION_KIND]: {
    className: "text-af-danger",
    iconKind: "exhaustion",
    label: getActivityGraphMessages().workstationIconLabel(
      EXHAUSTION_WORKSTATION_KIND,
    ),
    semanticKind: EXHAUSTION_WORKSTATION_KIND,
  },
  [REPEATER_WORKSTATION_KIND]: {
    className: "text-af-info",
    iconKind: "repeater",
    label: getActivityGraphMessages().workstationIconLabel(
      REPEATER_WORKSTATION_KIND,
    ),
    semanticKind: REPEATER_WORKSTATION_KIND,
  },
  [STANDARD_WORKSTATION_KIND]: {
    className: "text-af-text-subtle",
    iconKind: "workstation",
    label: getActivityGraphMessages().workstationIconLabel(
      STANDARD_WORKSTATION_KIND,
    ),
    semanticKind: STANDARD_WORKSTATION_KIND,
  },
} satisfies Record<WorkstationSemanticKind, WorkstationIconMetadata>;

export const SUPPORTED_WORKSTATION_ICON_METADATA =
  SUPPORTED_WORKSTATION_ICON_KINDS.map(
    (kind) => WORKSTATION_ICON_METADATA_BY_KIND[kind],
  );
export const EXHAUSTION_WORKSTATION_ICON_METADATA =
  WORKSTATION_ICON_METADATA_BY_KIND[EXHAUSTION_WORKSTATION_KIND];

function normalizeApiWorkstationKind(
  workstationKind: string | undefined,
): ApiWorkstationKind | null {
  const normalized = workstationKind?.trim().toUpperCase();
  switch (normalized) {
    case STANDARD_WORKSTATION_KIND:
    case REPEATER_WORKSTATION_KIND:
    case CRON_WORKSTATION_KIND:
    case POLLER_WORKSTATION_KIND:
      return normalized;
    default:
      return null;
  }
}

export function workstationSemanticKind(
  workstation: DashboardWorkstationNode,
): WorkstationSemanticKind {
  if (isExhaustionWorkstation(workstation)) {
    return EXHAUSTION_WORKSTATION_KIND;
  }
  return (
    normalizeApiWorkstationKind(workstation.workstation_kind) ??
    STANDARD_WORKSTATION_KIND
  );
}

export function workstationIconMetadata(
  workstation: DashboardWorkstationNode,
  locale?: string | null,
): WorkstationIconMetadata {
  const metadata =
    WORKSTATION_ICON_METADATA_BY_KIND[workstationSemanticKind(workstation)];

  return {
    ...metadata,
    label: getActivityGraphMessages(locale).workstationIconLabel(
      metadata.semanticKind,
    ),
  };
}
