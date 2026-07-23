import type { DashboardSnapshot } from "../../../../../api/dashboard/types";
import type { SessionFactoryDocument } from "../../../../../api/session-factory";
import { singleNodeDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import {
  defaultSessionFactoryVersion,
  incrementSessionFactoryVersion,
  parseSessionFactoryPutFactory,
} from "../../../../../testing/session-factory-mocks";
import { createDefaultFactoryLayout } from "../../../lib/layout/factory-graph-layout-operations";

const VISUAL_GROUP_EDITOR_STORAGE_KEY =
  "factory-graph-visual-group-editor-persisted-factory";

export function createVisualGroupEditorFactoryDocument(): SessionFactoryDocument {
  const factory = singleNodeDashboardSnapshot.factory;
  if (!factory) {
    throw new Error("Expected single-node dashboard snapshot factory.");
  }

  return {
    ...factory,
    layout: createDefaultFactoryLayout(),
    version: { ...defaultSessionFactoryVersion },
  };
}

let persistedFactoryDocument = createVisualGroupEditorFactoryDocument();

function readPersistedFactoryFromStorage(): SessionFactoryDocument | null {
  if (typeof localStorage === "undefined") {
    return null;
  }

  const raw = localStorage.getItem(VISUAL_GROUP_EDITOR_STORAGE_KEY);
  if (!raw) {
    return null;
  }

  return JSON.parse(raw) as SessionFactoryDocument;
}

function writePersistedFactoryToStorage(factory: SessionFactoryDocument): void {
  if (typeof localStorage === "undefined") {
    return;
  }

  localStorage.setItem(
    VISUAL_GROUP_EDITOR_STORAGE_KEY,
    JSON.stringify(factory),
  );
}

function clearPersistedFactoryStorage(): void {
  if (typeof localStorage === "undefined") {
    return;
  }

  localStorage.removeItem(VISUAL_GROUP_EDITOR_STORAGE_KEY);
}

export function resetVisualGroupEditorStoryState(): void {
  clearPersistedFactoryStorage();
  persistedFactoryDocument = createVisualGroupEditorFactoryDocument();
}

export function getVisualGroupEditorPersistedFactory(): SessionFactoryDocument {
  return readPersistedFactoryFromStorage() ?? persistedFactoryDocument;
}

export function adoptVisualGroupEditorPersistedFactory(
  factory: SessionFactoryDocument,
): void {
  persistedFactoryDocument = factory;
  writePersistedFactoryToStorage(factory);
}

export function buildVisualGroupEditorSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(singleNodeDashboardSnapshot);
  snapshot.factory = getVisualGroupEditorPersistedFactory();
  snapshot.factory_state = "IDLE";
  snapshot.runtime.in_flight_dispatch_count = 0;
  return snapshot;
}

export function buildVisualGroupEditorFetchMocks() {
  return [
    {
      method: "GET",
      path: "/factory-sessions/~default/factory",
      response: () => ({
        body: getVisualGroupEditorPersistedFactory(),
      }),
    },
    {
      method: "PUT",
      path: "/factory-sessions/~default/factory",
      response: (_input: RequestInfo | URL, init?: RequestInit) => {
        const savedFactory = parseSessionFactoryPutFactory(
          String(init?.body ?? "{}"),
        );
        const nextDocument: SessionFactoryDocument = {
          ...savedFactory,
          version: incrementSessionFactoryVersion(
            savedFactory.version ?? defaultSessionFactoryVersion,
          ),
        };
        adoptVisualGroupEditorPersistedFactory(nextDocument);
        return {
          body: nextDocument,
        };
      },
    },
  ];
}

declare global {
  interface Window {
    __resetVisualGroupEditorStory?: () => void;
    __getVisualGroupEditorPersistedFactory?: () => SessionFactoryDocument;
  }
}

if (typeof window !== "undefined") {
  window.__resetVisualGroupEditorStory = resetVisualGroupEditorStoryState;
  window.__getVisualGroupEditorPersistedFactory =
    getVisualGroupEditorPersistedFactory;
}
