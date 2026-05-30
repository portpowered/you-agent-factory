import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { FactoryValidationResult } from "../../../api/factory-validation";
import { createEmptyFactoryGraphDraft } from "../lib/factory-graph-draft-types";
import {
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
  removeFactoryGraphNode,
} from "../lib/factory-graph-operations";
import { baseFactoryDefinition } from "../lib/factory-graph-draft.test-helpers";
import { buildDraftAppliedFactoryDefinition } from "../lib/factory-graph-draft-apply";
import { useFactoryValidation } from "./use-factory-validation";

const validationFixtures = vi.hoisted(() => {
  const repeaterWithoutRejectRoute: FactoryValidationResult = {
    targets: [
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation repeater must define a reject route.",
        severity: "error",
        subject: {
          id: "repeater",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
    ],
  };
  const validFactory: FactoryValidationResult = {
    targets: [],
  };
  const reviewRemoved: FactoryValidationResult = {
    targets: [
      {
        code: "factory.route.danglingPlaceReference",
        message: "Workstation draft references a missing place.",
        severity: "error",
        subject: {
          id: "draft",
          location: "OUTPUTS",
          type: "WORKSTATION",
        },
      },
    ],
  };
  const disconnectedFailureRoute: FactoryValidationResult = {
    targets: [
      {
        code: "factory.workstation.missingFailureRoute",
        message: "Workstation draft must define a failure route.",
        severity: "error",
        subject: {
          id: "draft",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
    ],
  };

  return {
    disconnectedFailureRoute,
    repeaterWithoutRejectRoute,
    reviewRemoved,
    validFactory,
  };
});

vi.mock("../../../api/factory-validation", async () => {
  const actual = await vi.importActual<
    typeof import("../../../api/factory-validation")
  >("../../../api/factory-validation");

  return {
    ...actual,
    validateFactoryDefinition: vi.fn(),
  };
});

import { validateFactoryDefinition } from "../../../api/factory-validation";

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function renderValidationHook(
  queryClient: QueryClient,
  initialDefinition: ReturnType<typeof buildDraftAppliedFactoryDefinition>,
) {
  return renderHook(
    ({ definition }) =>
      useFactoryValidation(definition, true, { debounceMs: 0 }),
    {
      initialProps: {
        definition: initialDefinition,
      },
      wrapper: createWrapper(queryClient),
    },
  );
}

describe("useFactoryValidation debounce", () => {
  const debounceMs = 50;

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("waits for debounce before validating rapid draft changes", async () => {
    vi.mocked(validateFactoryDefinition).mockResolvedValue(
      validationFixtures.validFactory,
    );

    const queryClient = createQueryClient();
    const emptyDraftDefinition = buildDraftAppliedFactoryDefinition(
      baseFactoryDefinition,
      createEmptyFactoryGraphDraft(),
    );
    const repeaterDraft = createEmptyFactoryGraphDraft();
    repeaterDraft.additions.workstations.push({
      inputs: [],
      name: "repeater",
      outputs: [],
      type: "REPEATER_WORKSTATION",
      worker: "writer",
    });
    const repeaterDraftDefinition = buildDraftAppliedFactoryDefinition(
      baseFactoryDefinition,
      repeaterDraft,
    );

    const { rerender } = renderHook(
      ({ definition }) =>
        useFactoryValidation(definition, true, { debounceMs }),
      {
        initialProps: {
          definition: emptyDraftDefinition,
        },
        wrapper: createWrapper(queryClient),
      },
    );

    await waitFor(() => {
      expect(validateFactoryDefinition).toHaveBeenCalledTimes(1);
    });
    vi.mocked(validateFactoryDefinition).mockClear();

    rerender({
      definition: repeaterDraftDefinition,
    });

    expect(validateFactoryDefinition).not.toHaveBeenCalled();

    await waitFor(
      () => {
        expect(validateFactoryDefinition).toHaveBeenCalledTimes(1);
      },
      { timeout: debounceMs * 4 },
    );
    expect(validateFactoryDefinition).toHaveBeenCalledWith(
      repeaterDraftDefinition,
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });
});

describe("useFactoryValidation abort", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("aborts in-flight validation when a newer draft definition is requested", async () => {
    const firstDeferred = createDeferred<FactoryValidationResult>();
    const capturedSignals: AbortSignal[] = [];
    vi.mocked(validateFactoryDefinition)
      .mockImplementationOnce((_definition, options) => {
        if (options?.signal) {
          capturedSignals.push(options.signal);
        }
        return firstDeferred.promise;
      })
      .mockResolvedValueOnce(validationFixtures.validFactory);

    const queryClient = createQueryClient();
    const repeaterDraft = createEmptyFactoryGraphDraft();
    repeaterDraft.additions.workstations.push({
      inputs: [],
      name: "repeater",
      outputs: [],
      type: "REPEATER_WORKSTATION",
      worker: "writer",
    });

    const { rerender } = renderValidationHook(
      queryClient,
      buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        createEmptyFactoryGraphDraft(),
      ),
    );

    await waitFor(() => {
      expect(validateFactoryDefinition).toHaveBeenCalledTimes(1);
    });

    rerender({
      definition: buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        repeaterDraft,
      ),
    });

    await waitFor(() => {
      expect(validateFactoryDefinition).toHaveBeenCalledTimes(2);
    });

    expect(capturedSignals[0]?.aborted).toBe(true);
  });
});

describe("useFactoryValidation stale response handling", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("ignores stale validation responses after a newer draft definition is requested", async () => {
    const firstDeferred = createDeferred<FactoryValidationResult>();
    const secondDeferred = createDeferred<FactoryValidationResult>();
    vi.mocked(validateFactoryDefinition)
      .mockImplementationOnce(() => firstDeferred.promise)
      .mockImplementationOnce(() => secondDeferred.promise);

    const queryClient = createQueryClient();
    const repeaterDraft = createEmptyFactoryGraphDraft();
    repeaterDraft.additions.workstations.push({
      inputs: [],
      name: "repeater",
      outputs: [],
      type: "REPEATER_WORKSTATION",
      worker: "writer",
    });

    const { rerender, result } = renderValidationHook(
      queryClient,
      buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        repeaterDraft,
      ),
    );

    rerender({
      definition: buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        createEmptyFactoryGraphDraft(),
      ),
    });

    await act(async () => {
      firstDeferred.resolve(validationFixtures.repeaterWithoutRejectRoute);
      await firstDeferred.promise.catch(() => undefined);
    });

    expect(result.current.targets).toEqual([]);

    await act(async () => {
      secondDeferred.resolve(validationFixtures.validFactory);
      await secondDeferred.promise;
    });

    await waitFor(() => {
      expect(result.current.targets).toEqual([]);
    });
  });
});

describe("useFactoryValidation draft mutation refresh on add and remove", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("refreshes targets after an add operation changes the draft-applied factory", async () => {
    vi.mocked(validateFactoryDefinition)
      .mockResolvedValueOnce(validationFixtures.validFactory)
      .mockResolvedValueOnce(validationFixtures.repeaterWithoutRejectRoute);

    const queryClient = createQueryClient();
    const repeaterDraft = createEmptyFactoryGraphDraft();
    repeaterDraft.additions.workstations.push({
      inputs: [],
      name: "repeater",
      outputs: [],
      type: "REPEATER_WORKSTATION",
      worker: "writer",
    });

    const { rerender, result } = renderValidationHook(
      queryClient,
      buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        createEmptyFactoryGraphDraft(),
      ),
    );

    await waitFor(() => {
      expect(result.current.targets).toEqual([]);
    });

    rerender({
      definition: buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        repeaterDraft,
      ),
    });

    await waitFor(() => {
      expect(result.current.targets).toHaveLength(1);
      expect(result.current.targets[0]?.subject.location).toBe("ON_REJECTION");
    });
  });

  it("refreshes targets after a remove operation changes the draft-applied factory", async () => {
    vi.mocked(validateFactoryDefinition)
      .mockResolvedValueOnce(validationFixtures.validFactory)
      .mockResolvedValueOnce(validationFixtures.reviewRemoved);

    const queryClient = createQueryClient();
    const removeDraft = removeFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      nodeId: "workstation:draft",
    });
    expect(removeDraft.ok).toBe(true);

    const { rerender, result } = renderValidationHook(
      queryClient,
      buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        createEmptyFactoryGraphDraft(),
      ),
    );

    await waitFor(() => {
      expect(result.current.targets).toEqual([]);
    });

    rerender({
      definition: buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        expectOk(removeDraft).value,
      ),
    });

    await waitFor(() => {
      expect(result.current.targets).toHaveLength(1);
      expect(result.current.targets[0]?.code).toBe(
        "factory.route.danglingPlaceReference",
      );
    });
  });
});

describe("useFactoryValidation draft mutation refresh on connect and disconnect", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("refreshes targets after a connect operation changes the draft-applied factory", async () => {
    vi.mocked(validateFactoryDefinition)
      .mockResolvedValueOnce(validationFixtures.disconnectedFailureRoute)
      .mockResolvedValueOnce(validationFixtures.validFactory);

    const queryClient = createQueryClient();
    const disconnectedDraft = createEmptyFactoryGraphDraft();
    const connectedDraft = connectFactoryGraphNodes({
      baseFactoryDefinition,
      draft: disconnectedDraft,
      sourceAnchorId: "workstation-on-failure-source",
      sourceNodeId: "workstation:draft",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
    });
    expect(connectedDraft.ok).toBe(true);

    const { rerender, result } = renderValidationHook(
      queryClient,
      buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        disconnectedDraft,
      ),
    );

    await waitFor(() => {
      expect(result.current.targets).toHaveLength(1);
      expect(result.current.targets[0]?.subject.location).toBe("ON_FAILURE");
    });

    rerender({
      definition: buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        expectOk(connectedDraft).value,
      ),
    });

    await waitFor(() => {
      expect(result.current.targets).toEqual([]);
    });
  });

  it("refreshes targets after a disconnect operation changes the draft-applied factory", async () => {
    vi.mocked(validateFactoryDefinition)
      .mockResolvedValueOnce(validationFixtures.validFactory)
      .mockResolvedValueOnce(validationFixtures.disconnectedFailureRoute);

    const queryClient = createQueryClient();
    const connectedDraft = connectFactoryGraphNodes({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      sourceAnchorId: "workstation-on-failure-source",
      sourceNodeId: "workstation:draft",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
    });
    expect(connectedDraft.ok).toBe(true);
    const disconnectedDraft = disconnectFactoryGraphEdge({
      baseFactoryDefinition,
      draft: expectOk(connectedDraft).value,
      edgeId: "workstation-on-failure:workstation:draft->work-state:story:done",
    });
    expect(disconnectedDraft.ok).toBe(true);

    const { rerender, result } = renderValidationHook(
      queryClient,
      buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        expectOk(connectedDraft).value,
      ),
    );

    await waitFor(() => {
      expect(result.current.targets).toEqual([]);
    });

    rerender({
      definition: buildDraftAppliedFactoryDefinition(
        baseFactoryDefinition,
        expectOk(disconnectedDraft).value,
      ),
    });

    await waitFor(() => {
      expect(result.current.targets).toHaveLength(1);
      expect(result.current.targets[0]?.subject.location).toBe("ON_FAILURE");
    });
  });
});

function expectOk<T>(
  result:
    | {
        ok: true;
        value: T;
      }
    | {
        ok: false;
      },
): { ok: true; value: T } {
  expect(result.ok).toBe(true);
  return result as { ok: true; value: T };
}
