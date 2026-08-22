import type { AgentBentoLayoutItem } from "../../components/agent-bento";
import type { DashboardLayoutInstanceHighWaterMarks } from "../dashboardLayoutSchema";

const DASHBOARD_WIDGET_TYPE_PATTERN = /^[a-z0-9-]+$/;
const DASHBOARD_WIDGET_INSTANCE_ID_PATTERN = /^([a-z0-9-]+)::instance-(\d+)$/;

export function normalizeDashboardLayoutInstanceHighWaterMarks(
  value: unknown,
): Record<string, number> {
  if (!value || typeof value !== "object") {
    return {};
  }

  const normalized: Record<string, number> = {};
  for (const [widgetType, highWaterMark] of Object.entries(
    value as Record<string, unknown>,
  )) {
    if (
      DASHBOARD_WIDGET_TYPE_PATTERN.test(widgetType) &&
      typeof highWaterMark === "number" &&
      Number.isSafeInteger(highWaterMark) &&
      highWaterMark >= 0
    ) {
      normalized[widgetType] = highWaterMark;
    }
  }
  return normalized;
}

export function mergeDashboardLayoutInstanceHighWaterMarks(
  ...sources: readonly (DashboardLayoutInstanceHighWaterMarks | undefined)[]
): Record<string, number> {
  const merged: Record<string, number> = {};
  for (const source of sources) {
    for (const [widgetType, highWaterMark] of Object.entries(
      normalizeDashboardLayoutInstanceHighWaterMarks(source),
    )) {
      merged[widgetType] = Math.max(merged[widgetType] ?? 0, highWaterMark);
    }
  }
  return merged;
}

export function getDashboardLayoutInstanceHighWaterMarks(
  layout: readonly AgentBentoLayoutItem[],
  initialHighWaterMarks: DashboardLayoutInstanceHighWaterMarks = {},
): Record<string, number> {
  const highWaterMarks = mergeDashboardLayoutInstanceHighWaterMarks(
    initialHighWaterMarks,
  );

  for (const item of layout) {
    const match = item.id.match(DASHBOARD_WIDGET_INSTANCE_ID_PATTERN);
    if (!match || match[1] !== item.widgetType) {
      continue;
    }

    const instanceNumber = Number.parseInt(match[2] ?? "", 10);
    if (!Number.isSafeInteger(instanceNumber)) {
      continue;
    }

    highWaterMarks[item.widgetType] = Math.max(
      highWaterMarks[item.widgetType] ?? 0,
      instanceNumber,
    );
  }

  return highWaterMarks;
}
