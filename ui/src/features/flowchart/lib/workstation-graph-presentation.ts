import type { FactoryGraphVisualStatusRole } from "@you-agent-factory/factory-graph";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import type { EditableWorkstationBehavior } from "../../current-factory-definition/lib/workstation-behavior";
import {
  CRON_WORKSTATION_KIND,
  POLLER_WORKSTATION_KIND,
  REPEATER_WORKSTATION_KIND,
  STANDARD_WORKSTATION_KIND,
  UNKNOWN_WORKSTATION_KIND,
  type WorkstationIconMetadata,
  type WorkstationSemanticKind,
  workstationIconMetadata,
} from "./workstation-icon-metadata";

export interface WorkstationGraphPresentation extends WorkstationIconMetadata {
  borderClassName?: string;
}

export function workstationBehaviorSemanticKind(
  behavior: EditableWorkstationBehavior | undefined,
): WorkstationSemanticKind {
  switch (behavior) {
    case "CRON":
      return CRON_WORKSTATION_KIND;
    case "POLLER":
      return POLLER_WORKSTATION_KIND;
    case "REPEATER":
      return REPEATER_WORKSTATION_KIND;
    case "STANDARD":
      return STANDARD_WORKSTATION_KIND;
    case undefined:
      return UNKNOWN_WORKSTATION_KIND;
  }
}

export function workstationGraphBorderClassName(
  semanticKind: WorkstationSemanticKind,
): string | undefined {
  switch (semanticKind) {
    case REPEATER_WORKSTATION_KIND:
      return "border-double";
    case POLLER_WORKSTATION_KIND:
      return "border-dotted";
    case CRON_WORKSTATION_KIND:
      return "border-dashed";
    default:
      return undefined;
  }
}

export function workstationGraphPresentation(
  workstation: DashboardWorkstationNode,
  locale?: string | null,
  parentStatus?: FactoryGraphVisualStatusRole,
): WorkstationGraphPresentation {
  const metadata = workstationIconMetadata(workstation, locale, parentStatus);

  return {
    ...metadata,
    borderClassName: workstationGraphBorderClassName(metadata.semanticKind),
  };
}

export function workstationGraphPresentationFromBehavior(
  behavior: EditableWorkstationBehavior | undefined,
  locale?: string | null,
  parentStatus?: FactoryGraphVisualStatusRole,
): WorkstationGraphPresentation {
  return workstationGraphPresentation(
    {
      node_id: "",
      transition_id: "",
      workstation_kind: workstationBehaviorSemanticKind(behavior),
      workstation_name: "",
      worker_type: "",
    },
    locale,
    parentStatus,
  );
}
