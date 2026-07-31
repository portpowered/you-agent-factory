import type { GraphSemanticIconKind } from "./semantic-icon.js";
import { factoryGraphNodeTitleClassName } from "./semantic-node-style.js";

export interface FactoryGraphWorkstationRef {
  node_id: string;
  transition_id: string;
  workstation_name: string;
  worker_type?: string;
  workstation_kind?: string;
}

export interface FactoryGraphWorkItemRef {
  display_name?: string;
  work_id: string;
  work_type_id?: string;
}

export type FactoryGraphWorkstationSemanticKind =
  | "CRON"
  | "POLLER"
  | "REPEATER"
  | "STANDARD"
  | "exhaustion";

export interface FactoryGraphWorkstationPresentation {
  borderClassName?: string;
  className: string;
  iconKind: GraphSemanticIconKind;
  label: string;
  semanticKind: FactoryGraphWorkstationSemanticKind;
}

export function factoryGraphWorkstationPresentation(
  workstation: FactoryGraphWorkstationRef,
  locale?: string,
): FactoryGraphWorkstationPresentation {
  const exhaustion =
    workstation.workstation_kind === "exhaustion" ||
    (!workstation.workstation_kind && !workstation.worker_type);
  const normalized = workstation.workstation_kind?.trim().toUpperCase();
  const semanticKind: FactoryGraphWorkstationSemanticKind = exhaustion
    ? "exhaustion"
    : normalized === "CRON"
      ? "CRON"
      : normalized === "POLLER"
        ? "POLLER"
        : normalized === "REPEATER"
          ? "REPEATER"
          : "STANDARD";
  const chinese = locale === "zh-CN";
  const values: Record<
    FactoryGraphWorkstationSemanticKind,
    Omit<FactoryGraphWorkstationPresentation, "label">
  > = {
    CRON: {
      borderClassName: "border-dashed",
      className: "text-success",
      iconKind: "cron",
      semanticKind: "CRON",
    },
    POLLER: {
      borderClassName: "border-dotted",
      className: "text-primary",
      iconKind: "poller",
      semanticKind: "POLLER",
    },
    REPEATER: {
      borderClassName: "border-double",
      className: "text-info",
      iconKind: "repeater",
      semanticKind: "REPEATER",
    },
    STANDARD: {
      className: "text-on-surface-subtle",
      iconKind: "workstation",
      semanticKind: "STANDARD",
    },
    exhaustion: {
      className: "text-error",
      iconKind: "exhaustion",
      semanticKind: "exhaustion",
    },
  };
  const labels: Record<FactoryGraphWorkstationSemanticKind, string> = {
    CRON: chinese ? "Cron 工作站" : "Cron workstation",
    POLLER: chinese ? "轮询器工作站" : "Poller workstation",
    REPEATER: chinese ? "重复器工作站" : "Repeater workstation",
    STANDARD: chinese ? "标准工作站" : "Standard workstation",
    exhaustion: chinese ? "耗尽规则" : "Exhaustion rule",
  };
  return { ...values[semanticKind], label: labels[semanticKind] };
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

export function factoryGraphSelectExhaustionLabel(
  title: string,
  locale?: string,
): string {
  return locale === "zh-CN"
    ? `选择 ${title} 枯竭规则`
    : `Select ${title} exhaustion rule`;
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
