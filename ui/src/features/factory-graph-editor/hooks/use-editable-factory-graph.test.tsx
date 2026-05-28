import { act, renderHook } from "@testing-library/react";
import {
  baseFactoryDefinition,
  currentFactoryDocument,
} from "../lib/factory-graph-draft.test-helpers";
import {
  buildFactoryGraphTopologyFromDefinition,
  createEmptyFactoryGraphDraft,
} from "../public";
import { useEditableFactoryGraph } from "./use-editable-factory-graph";

const hookState = vi.hoisted(() => ({
  draftState: {} as ReturnType<
    typeof import("./factory-graph-draft-hook").useFactoryGraphDraftState
  >,
}));

vi.mock("./factory-graph-draft-hook", () => ({
  useFactoryGraphDraftState: () => hookState.draftState,
}));

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the view-model contract is clearer when its state and action scenarios stay together.
describe("useEditableFactoryGraph", () => {
  beforeEach(() => {
    const draft = createEmptyFactoryGraphDraft();
    hookState.draftState = {
      baseDocument: currentFactoryDocument,
      draft,
      graph: buildFactoryGraphTopologyFromDefinition(currentFactoryDocument),
      hasChanges: false,
      latestDocument: currentFactoryDocument,
      pendingFactoryDefinition: currentFactoryDocument,
      replaceDraft: vi.fn((nextDraft) => {
        hookState.draftState.draft = nextDraft;
        hookState.draftState.hasChanges = true;
      }),
      resetDraft: vi.fn(() => {
        hookState.draftState.draft = createEmptyFactoryGraphDraft();
        hookState.draftState.hasChanges = false;
      }),
      source: "current-factory",
      updateDraft: vi.fn(),
      validationErrors: [],
    };
  });

  it("exposes pending, projection, validation, and blocked operation state", () => {
    const { result, rerender } = renderHook(() =>
      useEditableFactoryGraph({
        currentFactoryDocument,
      }),
    );

    expect(result.current.pendingState.hasChanges).toBe(false);
    expect(result.current.projection.nodes.map((node) => node.id)).toContain(
      "workstation:draft",
    );
    expect(result.current.validationState.isValid).toBe(true);

    act(() => {
      result.current.actions.addNode({
        kind: "worker",
        model: "gpt-5-mini",
        name: "reviewer",
      });
    });
    rerender();

    expect(result.current.pendingState.hasChanges).toBe(true);
    expect(
      result.current.graphState?.graph.nodes.map((node) => node.id),
    ).toContain("worker:reviewer");

    act(() => {
      result.current.actions.addNode({
        kind: "worker",
        model: "gpt-5-mini",
        name: "writer",
      });
    });

    expect(result.current.blockedOperation).toMatchObject({
      ok: false,
      reason: "DUPLICATE_IDENTIFIER",
    });
    expect(result.current.pendingState.hasChanges).toBe(true);
  });

  it("reports stale save state when the current factory changes during a draft", () => {
    hookState.draftState.hasChanges = true;
    hookState.draftState.latestDocument = {
      ...currentFactoryDocument,
      version: {
        logical: "6",
        physical: "2026-05-18T16:00:00Z",
      },
    };

    const { result } = renderHook(() =>
      useEditableFactoryGraph({
        currentFactoryDocument:
          hookState.draftState.latestDocument ?? undefined,
        saveFactoryDefinition: async () => undefined,
      }),
    );

    expect(result.current.saveState.isStale).toBe(true);
    expect(result.current.saveState.canSave).toBe(false);
  });

  it("saves pending edits through the provided save callback", async () => {
    const saveFactoryDefinition = vi.fn(async () => undefined);
    hookState.draftState.hasChanges = true;
    hookState.draftState.draft = {
      ...createEmptyFactoryGraphDraft(),
      additions: {
        ...createEmptyFactoryGraphDraft().additions,
        resources: [
          {
            capacity: 1,
            name: "review-slot",
          },
        ],
      },
    };

    const { result } = renderHook(() =>
      useEditableFactoryGraph({
        currentFactoryDocument,
        saveFactoryDefinition,
      }),
    );

    let didSave = false;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(true);
    expect(saveFactoryDefinition).toHaveBeenCalledWith({
      baseVersion: currentFactoryDocument.version,
      factoryDefinition: expect.objectContaining({
        resources: expect.arrayContaining([
          expect.objectContaining({ name: "review-slot" }),
        ]),
      }),
    });
    expect(hookState.draftState.replaceDraft).toHaveBeenCalledWith(
      createEmptyFactoryGraphDraft(),
    );
    expect(result.current.saveState.lastSuccess).toBe(true);
  });

  it("keeps the draft and exposes save errors when the save callback fails", async () => {
    const saveFactoryDefinition = vi.fn(async () => {
      throw new Error("API unavailable");
    });
    hookState.draftState.hasChanges = true;
    hookState.draftState.draft = {
      ...createEmptyFactoryGraphDraft(),
      additions: {
        ...createEmptyFactoryGraphDraft().additions,
        workers: [
          {
            model: "gpt-5-mini",
            name: "reviewer",
          },
        ],
      },
    };

    const { result } = renderHook(() =>
      useEditableFactoryGraph({
        currentFactoryDocument: {
          ...baseFactoryDefinition,
          version: currentFactoryDocument.version,
        },
        saveFactoryDefinition,
      }),
    );

    let didSave = true;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(false);
    expect(hookState.draftState.hasChanges).toBe(true);
    expect(result.current.saveState.lastError).toBe("API unavailable");
  });
});
