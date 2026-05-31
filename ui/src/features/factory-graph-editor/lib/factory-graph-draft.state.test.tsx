import { act, renderHook } from "@testing-library/react";

import { buildPendingFactoryDefinition } from "./factory-graph-draft-apply";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import { validateFactoryGraphDraft } from "./factory-graph-draft-validation";
import {
  syncFactoryGraphDraftSession,
  useFactoryGraphDraftState,
} from "../hooks/factory-graph-draft-hook";
import {
  baseFactoryDefinition,
  currentFactoryDocument,
} from "./factory-graph-draft.test-helpers";

it("replaces workstation worker assignments in the pending definition without introducing validation errors", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.additions.workers.push({
    model: "gpt-5-mini",
    name: "reviewer",
    type: "MODEL_WORKER",
  });
  draft.edgeChanges.removals.push({
    kind: "worker-assignment",
    source: {
      kind: "worker",
      name: "writer",
    },
    target: {
      kind: "workstation",
      name: "draft",
    },
  });
  draft.edgeChanges.additions.push({
    kind: "worker-assignment",
    source: {
      kind: "worker",
      name: "reviewer",
    },
    target: {
      kind: "workstation",
      name: "draft",
    },
  });

  const pendingFactoryDefinition = buildPendingFactoryDefinition(
    baseFactoryDefinition,
    draft,
  );
  const validationErrors = validateFactoryGraphDraft(
    baseFactoryDefinition,
    draft,
  );

  expect(
    pendingFactoryDefinition?.workstations?.find(
      (workstation) => workstation.name === "draft",
    ),
  ).toMatchObject({
    body: "Draft the story.",
    name: "draft",
    worker: "reviewer",
  });
  expect(validationErrors).not.toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
      }),
    ]),
  );
});

it("keeps a draft-applied pending definition while save validation blocks missing worker assignments", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.edgeChanges.removals.push({
    kind: "worker-assignment",
    source: {
      kind: "worker",
      name: "writer",
    },
    target: {
      kind: "workstation",
      name: "draft",
    },
  });

  const validationErrors = validateFactoryGraphDraft(
    baseFactoryDefinition,
    draft,
  );
  const pendingFactoryDefinition = buildPendingFactoryDefinition(
    baseFactoryDefinition,
    draft,
  );

  expect(validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
        target: {
          kind: "node",
          id: "workstation:draft",
        },
      }),
    ]),
  );
  expect(
    pendingFactoryDefinition?.workstations?.find(
      (workstation) => workstation.name === "draft",
    ),
  ).toMatchObject({
    name: "draft",
    worker: "",
  });
});

it("localizes interpolated validation errors for non-default locales", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.edgeChanges.removals.push({
    kind: "worker-assignment",
    source: {
      kind: "worker",
      name: "writer",
    },
    target: {
      kind: "workstation",
      name: "draft",
    },
  });

  const validationErrors = validateFactoryGraphDraft(
    baseFactoryDefinition,
    draft,
    "zh-CN",
  );

  expect(validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
        message: "工作站“draft”必须保留一个工作者分配。",
      }),
    ]),
  );
});

it("adopts a newer document version after save when the draft has no pending edits", () => {
  const synced = syncFactoryGraphDraftSession(
    {
      draft: createEmptyFactoryGraphDraft(),
      latestDocument: currentFactoryDocument,
      sessionStartDocument: currentFactoryDocument,
    },
    {
      ...currentFactoryDocument,
      version: {
        logical: "6",
        physical: "2026-05-18T15:05:00Z",
      },
    },
  );

  expect(synced.draft.additions.workers).toEqual([]);
  expect(synced.latestDocument.version.logical).toBe("6");
  expect(synced.sessionStartDocument.version.logical).toBe("6");
});

it("keeps a dirty draft while newer editable-definition versions arrive", () => {
  const dirtyDraft = createEmptyFactoryGraphDraft();
  dirtyDraft.additions.workers.push({
    model: "gpt-5-mini",
    name: "reviewer",
    type: "MODEL_WORKER",
  });

  const synced = syncFactoryGraphDraftSession(
    {
      draft: dirtyDraft,
      latestDocument: currentFactoryDocument,
      sessionStartDocument: currentFactoryDocument,
    },
    {
      ...baseFactoryDefinition,
      version: {
        logical: "6",
        physical: "2026-05-18T15:05:00Z",
      },
    },
  );

  expect(synced.draft.additions.workers).toEqual(dirtyDraft.additions.workers);
  expect(synced.sessionStartDocument.version.logical).toBe("5");
  expect(synced.latestDocument.version.logical).toBe("6");
});

it("clears draft state when the factory document scope key changes", () => {
  const { result, rerender } = renderHook(
    ({ factoryDocumentScopeKey }: { factoryDocumentScopeKey: string }) =>
      useFactoryGraphDraftState({
        currentFactoryDocument,
        factoryDocumentScopeKey,
      }),
    {
      initialProps: { factoryDocumentScopeKey: "session-alpha" },
    },
  );

  act(() => {
    result.current.replaceDraft({
      ...createEmptyFactoryGraphDraft(),
      additions: {
        resources: [],
        workers: [
          {
            model: "gpt-5-mini",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workStates: [],
        workTypes: [],
        workstations: [],
      },
    });
  });

  expect(result.current.hasChanges).toBe(true);

  rerender({ factoryDocumentScopeKey: "session-beta" });

  expect(result.current.hasChanges).toBe(false);
  expect(result.current.source).toBe("current-factory");
  expect(result.current.latestDocument).toEqual(currentFactoryDocument);
  expect(result.current.baseDocument).toEqual(currentFactoryDocument);
});

it("resets draft state when the loaded factory document identity changes", () => {
  const dirtyDraft = createEmptyFactoryGraphDraft();
  dirtyDraft.additions.workers.push({
    model: "gpt-5-mini",
    name: "reviewer",
    type: "MODEL_WORKER",
  });

  const synced = syncFactoryGraphDraftSession(
    {
      draft: dirtyDraft,
      latestDocument: currentFactoryDocument,
      sessionStartDocument: currentFactoryDocument,
    },
    {
      ...currentFactoryDocument,
      name: "Other Session Factory",
    },
  );

  expect(synced.draft.additions.workers).toEqual([]);
  expect(synced.latestDocument.name).toBe("Other Session Factory");
});

it("resets a dirty draft back to the latest server-backed document", () => {
  const { result } = renderHook(() =>
    useFactoryGraphDraftState({
      currentFactoryDocument,
    }),
  );

  act(() => {
    result.current.replaceDraft({
      ...createEmptyFactoryGraphDraft(),
      additions: {
        resources: [],
        workers: [
          {
            model: "gpt-5-mini",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workStates: [],
        workTypes: [],
        workstations: [],
      },
    });
  });

  expect(result.current.hasChanges).toBe(true);

  act(() => {
    result.current.resetDraft();
  });

  expect(result.current.hasChanges).toBe(false);
  expect(result.current.baseDocument?.version.logical).toBe("5");
  expect(result.current.latestDocument?.version.logical).toBe("5");
});
