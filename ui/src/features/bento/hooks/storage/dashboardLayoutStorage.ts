import type { AgentBentoLayoutItem } from "../../components/agent-bento";
import { mergeDashboardLayout } from "../dashboardLayoutPersistence";
import {
  DASHBOARD_LAYOUT_STORAGE_KEY,
  DASHBOARD_LAYOUT_STORAGE_VERSION,
  type DashboardLayoutScope,
  type DashboardLayoutStorageEnvelope,
  DEFAULT_DASHBOARD_LAYOUT,
  getDashboardLayoutStorageKey,
} from "../dashboardLayoutSchema";

export function readStoredDashboardLayout(
  scope?: DashboardLayoutScope | null,
): AgentBentoLayoutItem[] {
  try {
    const normalizedScope = scope ?? undefined;
    const storageKey = normalizedScope
      ? getDashboardLayoutStorageKey(normalizedScope)
      : DASHBOARD_LAYOUT_STORAGE_KEY;
    const storedLayout = window.localStorage.getItem(storageKey);
    if (storedLayout !== null) {
      const parsedLayout = JSON.parse(storedLayout);
      const layout = normalizedScope
        ? readScopedDashboardLayout(parsedLayout, normalizedScope)
        : readLegacyDashboardLayout(parsedLayout);
      if (!layout) {
        return DEFAULT_DASHBOARD_LAYOUT;
      }

      const normalizedLayout = mergeDashboardLayout(layout);
      if (normalizedScope) {
        writeStoredDashboardLayout(normalizedLayout, normalizedScope);
      }
      return normalizedLayout;
    }

    if (!normalizedScope) {
      return DEFAULT_DASHBOARD_LAYOUT;
    }

    const legacyStoredLayout = window.localStorage.getItem(
      DASHBOARD_LAYOUT_STORAGE_KEY,
    );
    if (!legacyStoredLayout) {
      return DEFAULT_DASHBOARD_LAYOUT;
    }

    const parsedLegacyLayout: unknown = JSON.parse(legacyStoredLayout);
    const legacyLayout = readLegacyDashboardLayout(parsedLegacyLayout);
    if (!legacyLayout) {
      return DEFAULT_DASHBOARD_LAYOUT;
    }

    const migratedLayout = mergeDashboardLayout(legacyLayout);
    writeStoredDashboardLayout(migratedLayout, normalizedScope);
    return migratedLayout;
  } catch {
    return DEFAULT_DASHBOARD_LAYOUT;
  }
}

export function writeStoredDashboardLayout(
  layout: AgentBentoLayoutItem[],
  scope?: DashboardLayoutScope | null,
): void {
  try {
    const normalizedScope = scope ?? undefined;
    const storageKey = normalizedScope
      ? getDashboardLayoutStorageKey(normalizedScope)
      : DASHBOARD_LAYOUT_STORAGE_KEY;
    const value: AgentBentoLayoutItem[] | DashboardLayoutStorageEnvelope =
      normalizedScope
        ? {
            layout,
            schemaVersion: DASHBOARD_LAYOUT_STORAGE_VERSION,
            scope: normalizedScope,
          }
        : layout;
    window.localStorage.setItem(storageKey, JSON.stringify(value));
  } catch {
    // Layout persistence is a convenience; dashboard interaction should keep working without it.
  }
}

function readLegacyDashboardLayout(
  value: unknown,
): AgentBentoLayoutItem[] | null {
  return Array.isArray(value) ? (value as AgentBentoLayoutItem[]) : null;
}

function readScopedDashboardLayout(
  value: unknown,
  scope: DashboardLayoutScope,
): AgentBentoLayoutItem[] | null {
  if (!isDashboardLayoutStorageEnvelope(value, scope)) {
    return null;
  }

  return value.layout;
}

function isDashboardLayoutStorageEnvelope(
  value: unknown,
  scope: DashboardLayoutScope,
): value is DashboardLayoutStorageEnvelope {
  if (!value || typeof value !== "object") {
    return false;
  }

  const candidate = value as {
    layout?: unknown;
    schemaVersion?: unknown;
    scope?: {
      factoryID?: unknown;
      sessionID?: unknown;
    };
  };
  return (
    candidate.schemaVersion === DASHBOARD_LAYOUT_STORAGE_VERSION &&
    Array.isArray(candidate.layout) &&
    candidate.scope?.factoryID === scope.factoryID &&
    candidate.scope?.sessionID === scope.sessionID
  );
}
