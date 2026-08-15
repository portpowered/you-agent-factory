import type { AgentBentoLayoutItem } from "../components/agent-bento";
import { isDuplicateCapableDashboardWidgetType } from "../lib/dashboard-widget-picker";
import {
  createDashboardWidgetInstanceID,
  DASHBOARD_WIDGET_IDS,
  type DashboardLayoutDiagnostic,
  type DashboardLayoutDiagnosticCode,
  getDefaultWidgetLayoutByType,
  isDashboardWidgetID,
  LEGACY_SELECTION_WIDGET_IDS,
  LEGACY_WORK_OUTCOME_WIDGET_IDS,
} from "./dashboardLayoutSchema";

const DASHBOARD_GRID_COLUMNS = 12;
const DASHBOARD_MAX_GRID_ROWS = 1000;
const DASHBOARD_WIDGET_INSTANCE_ID_PATTERN = /^[a-z0-9-]+::[a-z0-9-]+$/;

export interface DashboardLayoutSanitizationResult {
  diagnostics: DashboardLayoutDiagnostic[];
  layout: AgentBentoLayoutItem[];
}

class DashboardLayoutDiagnosticCollector {
  private readonly counts = new Map<DashboardLayoutDiagnosticCode, number>();

  constructor(initialDiagnostics: readonly DashboardLayoutDiagnostic[] = []) {
    for (const diagnostic of initialDiagnostics) {
      this.counts.set(
        diagnostic.code,
        (this.counts.get(diagnostic.code) ?? 0) + diagnostic.count,
      );
    }
  }

  add(code: DashboardLayoutDiagnosticCode) {
    this.counts.set(code, (this.counts.get(code) ?? 0) + 1);
  }

  toArray(): DashboardLayoutDiagnostic[] {
    return [...this.counts.entries()].map(([code, count]) => ({
      code,
      count,
      severity: isStorageDiagnostic(code) ? "error" : "repair",
    }));
  }
}

export function collectStoredLayoutDiagnostics(
  layout: readonly unknown[],
): DashboardLayoutDiagnostic[] {
  const diagnostics = new DashboardLayoutDiagnosticCollector();
  for (const value of layout) {
    if (!value || typeof value !== "object") {
      diagnostics.add("invalid-item");
      continue;
    }

    const candidate = value as Record<string, unknown>;
    const widgetType = resolveStoredWidgetType(
      candidate as Partial<AgentBentoLayoutItem> & { widgetType?: unknown },
    );
    if (!widgetType) {
      diagnostics.add("invalid-item");
      continue;
    }

    if (!isAcceptedStoredWidgetID(candidate.id, widgetType)) {
      diagnostics.add("invalid-id");
    }

    const defaultItem = getDefaultWidgetLayoutByType(widgetType);
    if (!defaultItem) {
      diagnostics.add("invalid-item");
      continue;
    }

    const width = candidate.w;
    const height = candidate.h;
    if (
      (width !== undefined &&
        (!isPositiveGridNumber(width) ||
          width < (defaultItem.minW ?? 1) ||
          width > DASHBOARD_GRID_COLUMNS)) ||
      (height !== undefined &&
        (!isPositiveGridNumber(height) || height > DASHBOARD_MAX_GRID_ROWS))
    ) {
      diagnostics.add("invalid-size");
    }

    const x = candidate.x;
    const y = candidate.y;
    const hasInvalidCoordinate =
      (x !== undefined && !isFiniteGridCoordinate(x)) ||
      (y !== undefined && !isFiniteGridCoordinate(y));
    const hasOutOfBoundsCoordinate =
      (typeof x === "number" &&
        typeof width === "number" &&
        x + width > DASHBOARD_GRID_COLUMNS) ||
      (typeof x === "number" && x < 0) ||
      (typeof y === "number" && y < 0) ||
      (typeof y === "number" && y > DASHBOARD_MAX_GRID_ROWS);
    if (hasInvalidCoordinate || hasOutOfBoundsCoordinate) {
      diagnostics.add("out-of-bounds");
    }
  }
  return diagnostics.toArray();
}

export function isSafeDashboardWidgetInstanceID(
  value: unknown,
  widgetType: string,
): value is string {
  return (
    typeof value === "string" &&
    value.length <= 160 &&
    DASHBOARD_WIDGET_INSTANCE_ID_PATTERN.test(value) &&
    value.startsWith(`${widgetType}::`)
  );
}

function isAcceptedStoredWidgetID(value: unknown, widgetType: string): boolean {
  if (isSafeDashboardWidgetInstanceID(value, widgetType)) {
    return true;
  }

  if (typeof value !== "string") {
    return false;
  }

  return (
    value === widgetType ||
    (widgetType === DASHBOARD_WIDGET_IDS.currentSelection &&
      LEGACY_SELECTION_WIDGET_IDS.some((legacyID) => legacyID === value)) ||
    (widgetType === DASHBOARD_WIDGET_IDS.workOutcomeChart &&
      LEGACY_WORK_OUTCOME_WIDGET_IDS.some((legacyID) => legacyID === value))
  );
}

export function repairDashboardLayout(
  layout: AgentBentoLayoutItem[],
  initialDiagnostics: readonly DashboardLayoutDiagnostic[] = [],
): DashboardLayoutSanitizationResult {
  const diagnostics = new DashboardLayoutDiagnosticCollector(
    initialDiagnostics,
  );
  const usedIDs = new Set<string>();
  const uniqueIDLayout = layout.map((item) => {
    if (isSafeDashboardWidgetInstanceID(item.id, item.widgetType)) {
      if (!usedIDs.has(item.id)) {
        usedIDs.add(item.id);
        return item;
      }

      diagnostics.add("duplicate-id");
    } else {
      diagnostics.add("invalid-id");
    }

    const repairedID = getNextRepairedWidgetInstanceID(
      item.widgetType,
      usedIDs,
    );
    usedIDs.add(repairedID);
    return { ...item, id: repairedID };
  });

  const retainedSingletonLayout: AgentBentoLayoutItem[] = [];
  const visibleWidgetTypes = new Set<string>();
  for (const item of uniqueIDLayout) {
    if (
      !item.hidden &&
      !isDuplicateCapableDashboardWidgetType(item.widgetType)
    ) {
      if (visibleWidgetTypes.has(item.widgetType)) {
        diagnostics.add("singleton-violation");
        continue;
      }
      visibleWidgetTypes.add(item.widgetType);
    }
    retainedSingletonLayout.push(item);
  }

  const repairedGeometry = retainedSingletonLayout.map((item) =>
    repairDashboardLayoutGeometry(item, diagnostics),
  );
  const repairedLayout = repairDashboardLayoutCollisions(
    repairedGeometry,
    diagnostics,
  );
  return {
    diagnostics: diagnostics.toArray(),
    layout: repairedLayout,
  };
}

function isStorageDiagnostic(code: DashboardLayoutDiagnosticCode): boolean {
  return (
    code === "malformed-json" ||
    code === "storage-quota-exceeded" ||
    code === "storage-read-failed" ||
    code === "storage-unavailable" ||
    code === "storage-write-failed" ||
    code === "unsupported-envelope"
  );
}

function getNextRepairedWidgetInstanceID(
  widgetType: string,
  usedIDs: ReadonlySet<string>,
): string {
  let instanceNumber = 1;
  let candidate = createDashboardWidgetInstanceID(
    widgetType,
    `instance-${instanceNumber}`,
  );
  while (usedIDs.has(candidate)) {
    instanceNumber += 1;
    candidate = createDashboardWidgetInstanceID(
      widgetType,
      `instance-${instanceNumber}`,
    );
  }
  return candidate;
}

function repairDashboardLayoutGeometry(
  item: AgentBentoLayoutItem,
  diagnostics: DashboardLayoutDiagnosticCollector,
): AgentBentoLayoutItem {
  const defaultItem = getDefaultWidgetLayoutByType(item.widgetType);
  const defaultWidth = defaultItem?.w ?? 1;
  const defaultHeight = defaultItem?.h ?? 1;
  const minWidth = defaultItem?.minW ?? 1;
  const minHeight = defaultItem?.minH ?? 1;

  let width = item.w;
  let height = item.h;
  let changedSize = false;
  if (!isPositiveGridNumber(width)) {
    width = defaultWidth;
    changedSize = true;
  }
  if (!isPositiveGridNumber(height)) {
    height = defaultHeight;
    changedSize = true;
  }

  const boundedWidth = Math.min(
    Math.max(width, minWidth),
    DASHBOARD_GRID_COLUMNS,
  );
  const boundedHeight = Math.min(
    Math.max(height, minHeight),
    DASHBOARD_MAX_GRID_ROWS,
  );
  if (boundedWidth !== width || boundedHeight !== height) {
    changedSize = true;
  }
  width = boundedWidth;
  height = boundedHeight;
  if (changedSize) {
    diagnostics.add("invalid-size");
  }

  let x = item.x;
  let y = item.y;
  let changedCoordinates = false;
  if (!isFiniteGridCoordinate(x)) {
    x = defaultItem?.x ?? 0;
    changedCoordinates = true;
  }
  if (!isFiniteGridCoordinate(y)) {
    y = defaultItem?.y ?? 0;
    changedCoordinates = true;
  }

  const boundedX = Math.min(Math.max(x, 0), DASHBOARD_GRID_COLUMNS - width);
  const boundedY = Math.min(Math.max(y, 0), DASHBOARD_MAX_GRID_ROWS - height);
  if (boundedX !== x || boundedY !== y) {
    changedCoordinates = true;
  }
  if (changedCoordinates) {
    diagnostics.add("out-of-bounds");
  }

  return {
    ...item,
    h: height,
    w: width,
    x: boundedX,
    y: boundedY,
  };
}

function repairDashboardLayoutCollisions(
  layout: AgentBentoLayoutItem[],
  diagnostics: DashboardLayoutDiagnosticCollector,
): AgentBentoLayoutItem[] {
  const placedItems: AgentBentoLayoutItem[] = [];
  return layout.map((item) => {
    if (item.hidden) {
      return item;
    }

    if (
      !placedItems.some((placedItem) =>
        dashboardLayoutItemsOverlap(placedItem, item),
      )
    ) {
      placedItems.push(item);
      return item;
    }

    diagnostics.add("collision");
    const position = findNextOpenDashboardPosition(placedItems, item, item);
    const repairedItem = { ...item, ...position };
    placedItems.push(repairedItem);
    return repairedItem;
  });
}

function findNextOpenDashboardPosition(
  layout: readonly AgentBentoLayoutItem[],
  item: Pick<AgentBentoLayoutItem, "h" | "w">,
  preferredPosition: Pick<AgentBentoLayoutItem, "x" | "y">,
): Pick<AgentBentoLayoutItem, "x" | "y"> {
  const maxX = Math.max(0, DASHBOARD_GRID_COLUMNS - item.w);
  const preferredX = Math.min(Math.max(preferredPosition.x, 0), maxX);
  const preferredY = Math.min(
    Math.max(preferredPosition.y, 0),
    DASHBOARD_MAX_GRID_ROWS - item.h,
  );
  const layoutBottom = Math.min(
    layout.reduce(
      (highestY, layoutItem) => Math.max(highestY, layoutItem.y + layoutItem.h),
      0,
    ) + item.h,
    DASHBOARD_MAX_GRID_ROWS,
  );

  for (let y = preferredY; y <= layoutBottom; y += 1) {
    const xCandidates =
      y === preferredY
        ? [
            preferredX,
            ...getGridColumnCandidates(maxX).filter((x) => x !== preferredX),
          ]
        : getGridColumnCandidates(maxX);

    for (const x of xCandidates) {
      const candidate = { h: item.h, w: item.w, x, y };
      if (
        !layout.some((layoutItem) =>
          dashboardLayoutItemsOverlap(layoutItem, candidate),
        )
      ) {
        return { x, y };
      }
    }
  }

  return { x: 0, y: Math.min(layoutBottom, DASHBOARD_MAX_GRID_ROWS - item.h) };
}

function getGridColumnCandidates(maxX: number): number[] {
  return Array.from({ length: maxX + 1 }, (_, index) => index);
}

function dashboardLayoutItemsOverlap(
  left: Pick<AgentBentoLayoutItem, "h" | "w" | "x" | "y">,
  right: Pick<AgentBentoLayoutItem, "h" | "w" | "x" | "y">,
): boolean {
  return !(
    left.x + left.w <= right.x ||
    right.x + right.w <= left.x ||
    left.y + left.h <= right.y ||
    right.y + right.h <= left.y
  );
}

export function resolveStoredWidgetType(
  item: Partial<AgentBentoLayoutItem> & { widgetType?: unknown },
): string | null {
  if (isDashboardWidgetID(item.widgetType)) {
    return item.widgetType;
  }

  if (typeof item.id !== "string") {
    return null;
  }
  const itemID = item.id;

  switch (itemID) {
    case DASHBOARD_WIDGET_IDS.currentSelection:
      return DASHBOARD_WIDGET_IDS.currentSelection;
    case DASHBOARD_WIDGET_IDS.providerSession:
      return DASHBOARD_WIDGET_IDS.providerSession;
    case DASHBOARD_WIDGET_IDS.submitWork:
      return DASHBOARD_WIDGET_IDS.submitWork;
    case DASHBOARD_WIDGET_IDS.terminalWork:
      return DASHBOARD_WIDGET_IDS.terminalWork;
    case DASHBOARD_WIDGET_IDS.trace:
      return DASHBOARD_WIDGET_IDS.trace;
    case DASHBOARD_WIDGET_IDS.workerSessionTimeline:
      return DASHBOARD_WIDGET_IDS.workerSessionTimeline;
    case DASHBOARD_WIDGET_IDS.workGraph:
      return DASHBOARD_WIDGET_IDS.workGraph;
    case DASHBOARD_WIDGET_IDS.workOutcomeChart:
      return DASHBOARD_WIDGET_IDS.workOutcomeChart;
    case DASHBOARD_WIDGET_IDS.workTotals:
      return DASHBOARD_WIDGET_IDS.workTotals;
    case DASHBOARD_WIDGET_IDS.addWidget:
      return DASHBOARD_WIDGET_IDS.addWidget;
    default:
      break;
  }

  if (LEGACY_SELECTION_WIDGET_IDS.some((legacyID) => legacyID === itemID)) {
    return DASHBOARD_WIDGET_IDS.currentSelection;
  }

  if (LEGACY_WORK_OUTCOME_WIDGET_IDS.some((legacyID) => legacyID === itemID)) {
    return DASHBOARD_WIDGET_IDS.workOutcomeChart;
  }

  if (itemID.startsWith(`${DASHBOARD_WIDGET_IDS.workOutcomeChart}::`)) {
    return DASHBOARD_WIDGET_IDS.workOutcomeChart;
  }

  if (itemID.startsWith(`${DASHBOARD_WIDGET_IDS.addWidget}::`)) {
    return DASHBOARD_WIDGET_IDS.addWidget;
  }

  const supportedWidgetID = Object.values(DASHBOARD_WIDGET_IDS).find(
    (widgetID) => itemID.startsWith(`${widgetID}::`),
  );
  return supportedWidgetID ?? null;
}

function isPositiveGridNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function isFiniteGridCoordinate(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value);
}
