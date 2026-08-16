import {
  type FactoryGraphVisualNestedAccentRole,
  type FactoryGraphVisualStatusRole,
  factoryGraphVisualNestedAccentRole,
} from "@you-agent-factory/factory-graph";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import {
  type components,
  WorkstationKind,
} from "../../../api/generated/openapi";
import type { GraphSemanticIconKind } from "../components/graph-semantic-icon";
import { getActivityGraphMessages } from "../messages/activity-graph";

export type ApiWorkstationKind = components["schemas"]["WorkstationKind"];

export const STANDARD_WORKSTATION_KIND =
  WorkstationKind.STANDARD satisfies ApiWorkstationKind;
export const REPEATER_WORKSTATION_KIND =
  WorkstationKind.REPEATER satisfies ApiWorkstationKind;
export const CRON_WORKSTATION_KIND =
  WorkstationKind.CRON satisfies ApiWorkstationKind;
export const POLLER_WORKSTATION_KIND =
  WorkstationKind.POLLER satisfies ApiWorkstationKind;
export const UNKNOWN_WORKSTATION_KIND = "UNKNOWN" as const;

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
  | typeof UNKNOWN_WORKSTATION_KIND;

export interface WorkstationIconMetadata {
  className: string;
  iconKind: GraphSemanticIconKind;
  label: string;
  semanticKind: WorkstationSemanticKind;
}

const NEUTRAL_WORKSTATION_ICON_CLASS_NAME = "text-on-surface-subtle";

const WORKSTATION_ICON_CLASS_NAME_BY_NESTED_ACCENT_ROLE: Record<
  FactoryGraphVisualNestedAccentRole,
  string
> = {
  neutral: "text-on-surface-variant",
  info: "text-info",
  warning: "text-warning",
  success: "text-success",
  danger: "text-error",
};

function workstationIconClassName(
  parentStatus?: FactoryGraphVisualStatusRole,
): string {
  if (parentStatus === undefined) {
    return NEUTRAL_WORKSTATION_ICON_CLASS_NAME;
  }

  return WORKSTATION_ICON_CLASS_NAME_BY_NESTED_ACCENT_ROLE[
    factoryGraphVisualNestedAccentRole(parentStatus)
  ];
}

const WORKSTATION_ICON_METADATA_BY_KIND = {
  [CRON_WORKSTATION_KIND]: {
    className: workstationIconClassName(),
    iconKind: "cron",
    label: getActivityGraphMessages().workstationIconLabel(
      CRON_WORKSTATION_KIND,
    ),
    semanticKind: CRON_WORKSTATION_KIND,
  },
  [POLLER_WORKSTATION_KIND]: {
    className: workstationIconClassName(),
    iconKind: "poller",
    label: getActivityGraphMessages().workstationIconLabel(
      POLLER_WORKSTATION_KIND,
    ),
    semanticKind: POLLER_WORKSTATION_KIND,
  },
  [REPEATER_WORKSTATION_KIND]: {
    className: workstationIconClassName(),
    iconKind: "repeater",
    label: getActivityGraphMessages().workstationIconLabel(
      REPEATER_WORKSTATION_KIND,
    ),
    semanticKind: REPEATER_WORKSTATION_KIND,
  },
  [STANDARD_WORKSTATION_KIND]: {
    className: workstationIconClassName(),
    iconKind: "workstation",
    label: getActivityGraphMessages().workstationIconLabel(
      STANDARD_WORKSTATION_KIND,
    ),
    semanticKind: STANDARD_WORKSTATION_KIND,
  },
  [UNKNOWN_WORKSTATION_KIND]: {
    className: workstationIconClassName(),
    iconKind: "workstation",
    label: getActivityGraphMessages().workstationIconLabel(
      UNKNOWN_WORKSTATION_KIND,
    ),
    semanticKind: UNKNOWN_WORKSTATION_KIND,
  },
} satisfies Record<WorkstationSemanticKind, WorkstationIconMetadata>;

export const SUPPORTED_WORKSTATION_ICON_METADATA =
  SUPPORTED_WORKSTATION_ICON_KINDS.map(
    (kind) => WORKSTATION_ICON_METADATA_BY_KIND[kind],
  );

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

/** Legacy dashboard adapter: absent or future topology metadata stays neutral. */
export function workstationSemanticKind(
  workstation: DashboardWorkstationNode,
): WorkstationSemanticKind {
  return (
    normalizeApiWorkstationKind(workstation.workstation_kind) ??
    UNKNOWN_WORKSTATION_KIND
  );
}

export function workstationIconMetadata(
  workstation: DashboardWorkstationNode,
  locale?: string | null,
  parentStatus?: FactoryGraphVisualStatusRole,
): WorkstationIconMetadata {
  const metadata =
    WORKSTATION_ICON_METADATA_BY_KIND[workstationSemanticKind(workstation)];

  return {
    ...metadata,
    className: workstationIconClassName(parentStatus),
    label: getActivityGraphMessages(locale).workstationIconLabel(
      metadata.semanticKind,
    ),
  };
}
