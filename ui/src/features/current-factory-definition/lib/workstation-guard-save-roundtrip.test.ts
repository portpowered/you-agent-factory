import {
  CURRENT_FACTORY_EDITOR_SAVE_MODE,
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
  saveFactoryForSessionDocument,
} from "../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import { normalizeFactoryDefinition } from "../../../api/factory-definition/api";
import { validateFactoryDefinition } from "../../../api/factory-validation";
import { validateEditableWorkstationDraft } from "../../current-selection/workstation-selection/hooks/use-editable-workstation-configuration-state";
import {
  applyEditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "./workstation-editable-values";

const reviewWorkstationNode: DashboardWorkstationNode = {
  model: "gpt-5",
  node_id: "review",
  transition_id: "review",
  workstation_kind: "MODEL_WORKSTATION",
  workstation_name: "Review",
};

function buildGuardedFactoryFixture(): CurrentFactoryDocument {
  return {
    name: "Guarded Factory",
    version: {
      logical: "7",
      physical: "2026-06-01T13:00:00Z",
    },
    workers: [
      {
        model: "gpt-5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "approved", type: "TERMINAL" },
          { name: "planned", type: "TERMINAL" },
        ],
      },
    ],
    workstations: [
      {
        body: "Plan work",
        id: "plan",
        inputs: [{ state: "queued", workType: "planItem" }],
        name: "Plan",
        outputs: [{ state: "planned", workType: "story" }],
        worker: "reviewer",
      },
      {
        body: "Review work",
        guards: [{ maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" }],
        id: "review",
        inputs: [
          { state: "queued", workType: "planItem" },
          {
            guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
            state: "queued",
            workType: "story",
          },
        ],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        worker: "reviewer",
      },
    ],
  };
}

function buildEditedVisitCountGuardDraft(
  editableValues: NonNullable<
    ReturnType<typeof resolveEditableWorkstationValues>
  >,
) {
  return {
    ...editableWorkstationDraftFromValues(editableValues),
    guards: [
      { maxVisits: 4, type: "VISIT_COUNT" as const, workstation: "Plan" },
    ],
    inputs: [
      { guards: [], state: "queued", workType: "planItem" },
      {
        guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" as const }],
        state: "queued",
        workType: "story",
      },
    ],
  };
}

function buildSavedDocumentVersion(
  normalizedFactory: CurrentFactoryDocument,
): CurrentFactoryDocument {
  return {
    ...normalizedFactory,
    version: {
      logical: "8",
      physical: "2026-06-01T14:00:00Z",
    },
  };
}

describe("workstation guard save round-trip", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("normalizes, validates, saves, and reloads edited workstation and input guards", async () => {
    const factory = buildGuardedFactoryFixture();
    const editableValues = resolveEditableWorkstationValues(
      factory,
      reviewWorkstationNode,
    );
    expect(editableValues).not.toBeNull();
    if (!editableValues) {
      return;
    }

    const editedDraft = buildEditedVisitCountGuardDraft(editableValues);

    expect(
      validateEditableWorkstationDraft(editedDraft, editableValues, {
        diagnostics: [],
        result: { diagnostics: [], valid: true },
        status: "ready",
      }),
    ).toEqual({});

    const pendingFactory = applyEditableWorkstationDraft(
      factory,
      reviewWorkstationNode,
      editedDraft,
    );
    expect(pendingFactory).not.toBeNull();
    if (!pendingFactory) {
      return;
    }

    const normalizedFactory = normalizeFactoryDefinition(pendingFactory);
    expect(normalizedFactory.workstations?.[1]).toMatchObject({
      guards: [{ maxVisits: 4, type: "VISIT_COUNT", workstation: "Plan" }],
      inputs: [
        { state: "queued", workType: "planItem" },
        {
          guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" }],
          state: "queued",
          workType: "story",
        },
      ],
    });

    const validationFetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ targets: [] }), {
        headers: { "Content-Type": "application/json" },
        status: 200,
        statusText: "OK",
      }),
    );
    const validationResult = await validateFactoryDefinition(
      normalizedFactory,
      {
        fetch: validationFetch,
      },
    );
    expect(validationFetch).toHaveBeenCalledWith(
      expect.stringContaining("/factory-validations"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(validationResult.targets).toEqual([]);

    const savedDocument = buildSavedDocumentVersion(normalizedFactory);
    const saveFetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(savedDocument), {
        headers: { "Content-Type": "application/json" },
        status: 200,
        statusText: "OK",
      }),
    );
    const saved = await saveFactoryForSessionDocument(
      {
        baseVersion: factory.version,
        factoryDefinition: normalizedFactory,
        mode: CURRENT_FACTORY_EDITOR_SAVE_MODE,
      },
      { fetch: saveFetch },
    );
    expect(saveFetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({ method: "PUT" }),
    );
    expect(saved.workstations?.[1]?.guards).toEqual([
      { maxVisits: 4, type: "VISIT_COUNT", workstation: "Plan" },
    ]);

    const reloadFetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(savedDocument), {
        headers: { "Content-Type": "application/json" },
        status: 200,
        statusText: "OK",
      }),
    );
    const reloaded = await getCurrentFactoryDocument({ fetch: reloadFetch });
    const reloadedValues = resolveEditableWorkstationValues(
      reloaded,
      reviewWorkstationNode,
    );

    expect(reloadedValues).toMatchObject({
      guards: [{ maxVisits: 4, type: "VISIT_COUNT", workstation: "Plan" }],
      inputs: [
        { guards: [], state: "queued", workType: "planItem" },
        {
          guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" }],
          state: "queued",
          workType: "story",
        },
      ],
    });
  });
});

describe("workstation guard MATCHES_FIELDS save round-trip", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("saves edited MATCHES_FIELDS inputKey without normalizing selector text", async () => {
    const factory = buildGuardedFactoryFixture();
    const editableValues = resolveEditableWorkstationValues(
      factory,
      reviewWorkstationNode,
    );
    expect(editableValues).not.toBeNull();
    if (!editableValues) {
      return;
    }

    const editedSelector = '.Tags["_last_output"]';
    const editedDraft = {
      ...editableWorkstationDraftFromValues(editableValues),
      guards: [
        {
          matchConfig: { inputKey: editedSelector },
          type: "MATCHES_FIELDS" as const,
        },
      ],
    };

    const pendingFactory = applyEditableWorkstationDraft(
      factory,
      reviewWorkstationNode,
      editedDraft,
    );
    expect(pendingFactory).not.toBeNull();
    if (!pendingFactory) {
      return;
    }

    const normalizedFactory = normalizeFactoryDefinition(pendingFactory);
    expect(normalizedFactory.workstations?.[1]?.guards).toEqual([
      {
        matchConfig: { inputKey: editedSelector },
        type: "MATCHES_FIELDS",
      },
    ]);

    const saveFetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify(buildSavedDocumentVersion(normalizedFactory)),
        {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        },
      ),
    );
    await saveFactoryForSessionDocument(
      {
        baseVersion: factory.version,
        factoryDefinition: normalizedFactory,
        mode: CURRENT_FACTORY_EDITOR_SAVE_MODE,
      },
      { fetch: saveFetch },
    );

    const saveRequestBody = JSON.parse(
      String(saveFetch.mock.calls[0]?.[1]?.body),
    );
    expect(
      saveRequestBody.factory.workstations[1].guards[0].matchConfig.inputKey,
    ).toBe(editedSelector);
  });
});
