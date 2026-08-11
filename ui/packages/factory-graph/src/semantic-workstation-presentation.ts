import { WorkstationKind } from "@you-agent-factory/client";
import type { GraphSemanticIconKind } from "./semantic-icon.js";
import { factoryGraphNodeTitleClassName } from "./semantic-node-style.js";
import {
  type FactoryGraphWorkstationRuntimeRole,
  type FactoryGraphWorkstationRuntimeType,
  type FactoryGraphWorkstationSchedulingBehavior,
  type FactoryGraphWorkstationSemantics,
  factoryGraphWorkstationRuntimeRole,
  UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS,
} from "./workstation-semantics.js";

export interface FactoryGraphWorkstationRef {
  node_id: string;
  transition_id: string;
  workstation_name: string;
  /** @deprecated Runtime topology metadata is not semantic authority. */
  worker_type?: string;
  /** @deprecated Runtime topology metadata is not semantic authority. */
  workstation_kind?: string;
}

export interface FactoryGraphWorkItemRef {
  display_name?: string;
  work_id: string;
  work_type_id?: string;
}

export interface FactoryGraphWorkstationPresentation
  extends FactoryGraphWorkstationSemantics {
  borderClassName?: string;
  className: string;
  iconKind: GraphSemanticIconKind;
  label: string;
}

/** Render-independent workstation presentation metadata for a graph node. */
export function factoryGraphWorkstationPresentation(
  semantics: FactoryGraphWorkstationSemantics = UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS,
  locale?: string,
): FactoryGraphWorkstationPresentation {
  const runtime = runtimePresentation(semantics.runtimeType, locale);
  return {
    ...semantics,
    ...schedulingPresentation(semantics.schedulingBehavior),
    className: runtime.className,
    iconKind: runtime.iconKind,
    label: runtime.label,
  };
}

function runtimePresentation(
  runtimeType: FactoryGraphWorkstationRuntimeType,
  locale?: string,
): { className: string; iconKind: GraphSemanticIconKind; label: string } {
  const chinese = locale === "zh-CN";
  const runtimeRole = factoryGraphWorkstationRuntimeRole(runtimeType);
  const labels: Record<FactoryGraphWorkstationRuntimeRole, string> = {
    AGENT: chinese ? "代理工作站" : "Agent workstation",
    CLASSIFIER: chinese ? "分类器工作站" : "Classifier workstation",
    INFERENCE: chinese ? "推理工作站" : "Inference workstation",
    LOGICAL_MOVE: chinese ? "逻辑移动工作站" : "Logical move workstation",
    POLLER: chinese ? "轮询运行工作站" : "Poller-run workstation",
    SCRIPT: chinese ? "脚本工作站" : "Script workstation",
    UNKNOWN: chinese ? "未知工作站语义" : "Unknown workstation semantics",
  };
  const iconKinds: Record<
    FactoryGraphWorkstationRuntimeRole,
    GraphSemanticIconKind
  > = {
    AGENT: "workstation",
    CLASSIFIER: "queue",
    INFERENCE: "processing",
    LOGICAL_MOVE: "workstation",
    POLLER: "poller",
    SCRIPT: "workstation",
    UNKNOWN: "workstation",
  };
  return {
    className: "text-on-surface-subtle",
    iconKind: iconKinds[runtimeRole],
    label: labels[runtimeRole],
  };
}

function schedulingPresentation(
  behavior: FactoryGraphWorkstationSchedulingBehavior,
): { borderClassName?: string } {
  switch (behavior) {
    case WorkstationKind.CRON:
      return { borderClassName: "border-dashed" };
    case WorkstationKind.POLLER:
      return { borderClassName: "border-dotted" };
    case WorkstationKind.REPEATER:
      return { borderClassName: "border-double" };
    case WorkstationKind.STANDARD:
    case "UNKNOWN":
      return {};
  }
}

export function factoryGraphWorkItemLabel(
  item: FactoryGraphWorkItemRef,
): string {
  return item.display_name?.trim() || item.work_id?.trim() || "Unknown work";
}

export function factoryGraphGraphDuration(
  startedAt: string,
  now: number,
  locale?: string,
): string {
  const millis = durationMillis(startedAt, now);
  if (millis === undefined) return "Unavailable";
  const [value, suffix] = graphDurationParts(millis, locale);
  return `${value}${suffix}`;
}

export function factoryGraphDurationText(
  startedAt: string,
  now: number,
  locale?: string,
): string {
  const millis = durationMillis(startedAt, now);
  if (millis === undefined) return "Unavailable";
  const seconds = Math.floor(millis / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const [hour, minute, second] =
    locale === "zh-CN" ? ["小时", "分", "秒"] : ["h", "m", "s"];
  if (hours > 0) return `${hours}${hour} ${minutes % 60}${minute}`;
  if (minutes > 0) return `${minutes}${minute} ${seconds % 60}${second}`;
  return `${seconds}${second}`;
}

export function factoryGraphActiveItemsLabel(
  count: number,
  locale?: string,
): string {
  return locale === "zh-CN"
    ? `${count} 个活动项`
    : `${count} active ${count === 1 ? "item" : "items"}`;
}

export function factoryGraphSelectWorkstationLabel(
  title: string,
  locale?: string,
): string {
  return locale === "zh-CN"
    ? `选择 ${title} 工作站`
    : `Select ${title} workstation`;
}

export function factoryGraphWorkstationTitleClassName(label: string): string {
  return factoryGraphNodeTitleClassName(
    factoryGraphClassNames(
      "basis-0 flex-1",
      label.length > 34
        ? "text-[0.78rem]"
        : label.length > 20
          ? "text-[0.88rem]"
          : "text-[1rem]",
    ),
  );
}

export function factoryGraphWorkItemLabelClassName(label: string): string {
  return factoryGraphClassNames(
    "block min-w-0 basis-0 flex-1 truncate whitespace-nowrap leading-tight",
    label.length > 58
      ? "text-[0.64rem]"
      : label.length > 38
        ? "text-[0.68rem]"
        : "text-[0.74rem]",
  );
}

export function factoryGraphClassNames(
  ...values: Array<string | false | null | undefined>
): string {
  return values.filter(Boolean).join(" ");
}

function durationMillis(startedAt: string, now: number): number | undefined {
  const started = Date.parse(startedAt);
  return Number.isNaN(started) ? undefined : Math.max(0, now - started);
}

function graphDurationParts(millis: number, locale?: string): [string, string] {
  const units = [
    [24 * 60 * 60 * 1000, locale === "zh-CN" ? "天" : "d"],
    [60 * 60 * 1000, locale === "zh-CN" ? "时" : "h"],
    [60 * 1000, locale === "zh-CN" ? "分" : "m"],
    [1000, locale === "zh-CN" ? "秒" : "s"],
  ] as const;
  const unit = units.find(([size]) => millis >= size) ?? units[3];
  return [
    new Intl.NumberFormat(locale).format(Math.floor(millis / unit[0])),
    unit[1],
  ];
}
