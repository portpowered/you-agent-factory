import type { AgentBentoLayoutItem } from "../../components/agent-bento";
import {
  getDashboardLayoutInstanceHighWaterMarks,
  mergeDashboardLayoutInstanceHighWaterMarks,
} from "../allocation/dashboardLayoutAllocation";
import { sanitizeDashboardLayout } from "../dashboardLayoutPersistence";
import {
  DASHBOARD_LAYOUT_STORAGE_KEY,
  DASHBOARD_LAYOUT_STORAGE_VERSION,
  type DashboardLayoutDiagnostic,
  type DashboardLayoutInstanceHighWaterMarks,
  type DashboardLayoutScope,
  type DashboardLayoutStorageEnvelope,
  DEFAULT_DASHBOARD_LAYOUT,
  getDashboardLayoutStorageKey,
} from "../dashboardLayoutSchema";

export interface DashboardLayoutStorageReadResult {
  diagnostics: DashboardLayoutDiagnostic[];
  instanceHighWaterMarks: Record<string, number>;
  layout: AgentBentoLayoutItem[];
}

export interface DashboardLayoutStorageWriteResult {
  diagnostics: DashboardLayoutDiagnostic[];
  instanceHighWaterMarks: Record<string, number>;
  persisted: boolean;
}

interface StorageReadValue {
  diagnostic?: DashboardLayoutDiagnostic;
  value: string | null;
}

export function readStoredDashboardLayout(
  scope?: DashboardLayoutScope | null,
): AgentBentoLayoutItem[] {
  return readStoredDashboardLayoutResult(scope).layout;
}

export function readStoredDashboardLayoutResult(
  scope?: DashboardLayoutScope | null,
): DashboardLayoutStorageReadResult {
  const normalizedScope = scope ?? undefined;
  const storageResult = getLocalStorage();
  if (!storageResult.storage) {
    return {
      diagnostics: storageResult.diagnostic ? [storageResult.diagnostic] : [],
      instanceHighWaterMarks: {},
      layout: DEFAULT_DASHBOARD_LAYOUT,
    };
  }

  const storageKey = normalizedScope
    ? getDashboardLayoutStorageKey(normalizedScope)
    : DASHBOARD_LAYOUT_STORAGE_KEY;
  const scopedRead = readStorageValue(storageResult.storage, storageKey);
  if (scopedRead.diagnostic) {
    return {
      diagnostics: [scopedRead.diagnostic],
      instanceHighWaterMarks: {},
      layout: DEFAULT_DASHBOARD_LAYOUT,
    };
  }

  if (scopedRead.value !== null) {
    return resolveStoredLayout(
      scopedRead.value,
      normalizedScope,
      storageResult.storage,
    );
  }

  if (!normalizedScope) {
    return {
      diagnostics: [],
      instanceHighWaterMarks: {},
      layout: DEFAULT_DASHBOARD_LAYOUT,
    };
  }

  const legacyRead = readStorageValue(
    storageResult.storage,
    DASHBOARD_LAYOUT_STORAGE_KEY,
  );
  if (legacyRead.diagnostic) {
    return {
      diagnostics: [legacyRead.diagnostic],
      instanceHighWaterMarks: {},
      layout: DEFAULT_DASHBOARD_LAYOUT,
    };
  }
  if (legacyRead.value === null) {
    return {
      diagnostics: [],
      instanceHighWaterMarks: {},
      layout: DEFAULT_DASHBOARD_LAYOUT,
    };
  }

  const legacyResult = resolveStoredLayout(
    legacyRead.value,
    undefined,
    storageResult.storage,
  );
  if (legacyResult.layout === DEFAULT_DASHBOARD_LAYOUT) {
    return legacyResult;
  }

  const writeResult = writeStoredDashboardLayoutValue(
    legacyResult.layout,
    normalizedScope,
    storageResult.storage,
    legacyResult.instanceHighWaterMarks,
  );
  return {
    diagnostics: combineDiagnostics(
      legacyResult.diagnostics,
      writeResult.diagnostics,
    ),
    instanceHighWaterMarks: writeResult.instanceHighWaterMarks,
    layout: legacyResult.layout,
  };
}

export function writeStoredDashboardLayout(
  layout: AgentBentoLayoutItem[],
  scope?: DashboardLayoutScope | null,
  instanceHighWaterMarks?: DashboardLayoutInstanceHighWaterMarks,
): DashboardLayoutStorageWriteResult {
  const storageResult = getLocalStorage();
  const persistedHighWaterMarks =
    scope && instanceHighWaterMarks === undefined && storageResult.storage
      ? readStoredInstanceHighWaterMarks(storageResult.storage, scope)
      : {};
  const sanitized = sanitizeDashboardLayout(
    layout,
    DEFAULT_DASHBOARD_LAYOUT,
    mergeDashboardLayoutInstanceHighWaterMarks(
      persistedHighWaterMarks,
      instanceHighWaterMarks,
    ),
  );
  if (!storageResult.storage) {
    return {
      diagnostics: combineDiagnostics(
        sanitized.diagnostics,
        storageResult.diagnostic ? [storageResult.diagnostic] : [],
      ),
      instanceHighWaterMarks: sanitized.instanceHighWaterMarks,
      persisted: false,
    };
  }

  const writeResult = writeStoredDashboardLayoutValue(
    sanitized.layout,
    scope ?? undefined,
    storageResult.storage,
    sanitized.instanceHighWaterMarks,
  );
  return {
    diagnostics: combineDiagnostics(
      sanitized.diagnostics,
      writeResult.diagnostics,
    ),
    instanceHighWaterMarks: writeResult.instanceHighWaterMarks,
    persisted: writeResult.persisted,
  };
}

function resolveStoredLayout(
  storedValue: string,
  scope: DashboardLayoutScope | undefined,
  storage: Storage,
): DashboardLayoutStorageReadResult {
  let parsedValue: unknown;
  try {
    parsedValue = JSON.parse(storedValue);
  } catch {
    return {
      diagnostics: [createDiagnostic("malformed-json")],
      instanceHighWaterMarks: {},
      layout: DEFAULT_DASHBOARD_LAYOUT,
    };
  }

  const storedLayout = scope
    ? readScopedDashboardLayout(parsedValue, scope)
    : readLegacyDashboardLayout(parsedValue);
  if (!storedLayout) {
    return {
      diagnostics: [createDiagnostic("unsupported-envelope")],
      instanceHighWaterMarks: {},
      layout: DEFAULT_DASHBOARD_LAYOUT,
    };
  }

  const sanitized = sanitizeDashboardLayout(
    storedLayout.layout,
    DEFAULT_DASHBOARD_LAYOUT,
    storedLayout.instanceHighWaterMarks,
  );
  const writeResult = scope
    ? writeStoredDashboardLayoutValue(
        sanitized.layout,
        scope,
        storage,
        sanitized.instanceHighWaterMarks,
      )
    : {
        diagnostics: [],
        instanceHighWaterMarks: sanitized.instanceHighWaterMarks,
        persisted: false,
      };
  return {
    diagnostics: combineDiagnostics(
      sanitized.diagnostics,
      writeResult.diagnostics,
    ),
    instanceHighWaterMarks: writeResult.instanceHighWaterMarks,
    layout: sanitized.layout,
  };
}

function writeStoredDashboardLayoutValue(
  layout: AgentBentoLayoutItem[],
  scope: DashboardLayoutScope | undefined,
  storage: Storage,
  instanceHighWaterMarks: DashboardLayoutInstanceHighWaterMarks,
): DashboardLayoutStorageWriteResult {
  const storageKey = scope
    ? getDashboardLayoutStorageKey(scope)
    : DASHBOARD_LAYOUT_STORAGE_KEY;
  const value: AgentBentoLayoutItem[] | DashboardLayoutStorageEnvelope = scope
    ? {
        layout,
        schemaVersion: DASHBOARD_LAYOUT_STORAGE_VERSION,
        scope,
        instanceHighWaterMarks,
      }
    : layout;

  try {
    storage.setItem(storageKey, JSON.stringify(value));
    return { diagnostics: [], instanceHighWaterMarks, persisted: true };
  } catch (error) {
    return {
      diagnostics: [createDiagnostic(classifyStorageWriteError(error))],
      instanceHighWaterMarks,
      persisted: false,
    };
  }
}

function getLocalStorage(): {
  diagnostic?: DashboardLayoutDiagnostic;
  storage: Storage | null;
} {
  try {
    if (typeof window === "undefined" || !window.localStorage) {
      return {
        diagnostic: createDiagnostic("storage-unavailable"),
        storage: null,
      };
    }
    return { storage: window.localStorage };
  } catch {
    return {
      diagnostic: createDiagnostic("storage-unavailable"),
      storage: null,
    };
  }
}

function readStorageValue(storage: Storage, key: string): StorageReadValue {
  try {
    return { value: storage.getItem(key) };
  } catch {
    return {
      diagnostic: createDiagnostic("storage-read-failed"),
      value: null,
    };
  }
}

function classifyStorageWriteError(
  error: unknown,
): "storage-quota-exceeded" | "storage-write-failed" {
  if (
    error instanceof DOMException &&
    (error.name === "QuotaExceededError" ||
      error.name === "NS_ERROR_DOM_QUOTA_REACHED")
  ) {
    return "storage-quota-exceeded";
  }

  if (
    error &&
    typeof error === "object" &&
    "name" in error &&
    ((error as { name?: unknown }).name === "QuotaExceededError" ||
      (error as { name?: unknown }).name === "NS_ERROR_DOM_QUOTA_REACHED")
  ) {
    return "storage-quota-exceeded";
  }

  return "storage-write-failed";
}

function createDiagnostic(
  code: DashboardLayoutDiagnostic["code"],
): DashboardLayoutDiagnostic {
  return {
    code,
    count: 1,
    severity:
      code === "malformed-json" ||
      code === "storage-quota-exceeded" ||
      code === "storage-read-failed" ||
      code === "storage-unavailable" ||
      code === "storage-write-failed" ||
      code === "unsupported-envelope"
        ? "error"
        : "repair",
  };
}

function combineDiagnostics(
  ...diagnosticLists: readonly DashboardLayoutDiagnostic[][]
): DashboardLayoutDiagnostic[] {
  const diagnosticsByCode = new Map<
    DashboardLayoutDiagnostic["code"],
    DashboardLayoutDiagnostic
  >();
  for (const diagnostics of diagnosticLists) {
    for (const diagnostic of diagnostics) {
      const previous = diagnosticsByCode.get(diagnostic.code);
      diagnosticsByCode.set(diagnostic.code, {
        code: diagnostic.code,
        count: (previous?.count ?? 0) + diagnostic.count,
        severity:
          previous?.severity === "error" || diagnostic.severity === "error"
            ? "error"
            : "repair",
      });
    }
  }
  return [...diagnosticsByCode.values()];
}

interface StoredDashboardLayout {
  instanceHighWaterMarks: Record<string, number>;
  layout: AgentBentoLayoutItem[];
}

function readLegacyDashboardLayout(
  value: unknown,
): StoredDashboardLayout | null {
  if (!Array.isArray(value)) {
    return null;
  }

  const layout = value as AgentBentoLayoutItem[];
  return {
    instanceHighWaterMarks: getDashboardLayoutInstanceHighWaterMarks(layout),
    layout,
  };
}

function readScopedDashboardLayout(
  value: unknown,
  scope: DashboardLayoutScope,
): StoredDashboardLayout | null {
  if (!isDashboardLayoutStorageEnvelope(value, scope)) {
    return null;
  }

  return {
    instanceHighWaterMarks: mergeDashboardLayoutInstanceHighWaterMarks(
      value.instanceHighWaterMarks,
      getDashboardLayoutInstanceHighWaterMarks(value.layout),
    ),
    layout: value.layout,
  };
}

function readStoredInstanceHighWaterMarks(
  storage: Storage,
  scope: DashboardLayoutScope,
): DashboardLayoutInstanceHighWaterMarks {
  const storedValue = readStorageValue(
    storage,
    getDashboardLayoutStorageKey(scope),
  ).value;
  if (storedValue === null) {
    return {};
  }

  try {
    const parsedValue: unknown = JSON.parse(storedValue);
    return (
      readScopedDashboardLayout(parsedValue, scope)?.instanceHighWaterMarks ??
      {}
    );
  } catch {
    return {};
  }
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
