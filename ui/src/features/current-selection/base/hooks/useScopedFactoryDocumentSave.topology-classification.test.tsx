import { act, renderHook, waitFor } from "@testing-library/react";

import { mockFactoryDocumentSave } from "../../../../testing/factory-document-save-mocks";
import {
  createScopedFactoryDocumentSaveQueryClientWrapper,
  defaultScopedFactoryDocumentSaveRequest,
  seedScopedFactoryDocumentSaveTestSession,
} from "../../../../testing/scoped-factory-document-save-test-helpers";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useScopedFactoryDocumentSave } from "./useScopedFactoryDocumentSave";

beforeEach(() => {
  seedScopedFactoryDocumentSaveTestSession();
  vi.restoreAllMocks();
});

describe("useScopedFactoryDocumentSave topology classification", () => {
  it("starts with lastSuccessfulSaveWasTopologyAffecting false", () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "idle" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    expect(result.current.lastSuccessfulSaveWasTopologyAffecting).toBe(false);
  });

  it("records topology-affecting saves using the shared graph topology classifier", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "work-type:story",
        }),
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    const previousFactory = {
      name: "Current Factory",
      workTypes: [
        {
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
      workers: [],
      workstations: [],
    };
    const nextFactory = {
      ...previousFactory,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
          ],
        },
      ],
    };

    await act(async () => {
      await result.current.saveNow({
        baseVersion: defaultScopedFactoryDocumentSaveRequest.baseVersion,
        factory: nextFactory,
        previousFactory,
        scopeKey: "work-type:story",
      });
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
    expect(result.current.lastSuccessfulSaveWasTopologyAffecting).toBe(true);
  });

  it("records non-topology saves as not topology-affecting", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    const factory = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
      workers: [],
      workstations: [],
    };
    const nextFactory = {
      ...factory,
      workTypes: [
        {
          handlingBehavior: ["PRIORITY"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    };

    await act(async () => {
      await result.current.saveNow({
        baseVersion: defaultScopedFactoryDocumentSaveRequest.baseVersion,
        factory: nextFactory,
        previousFactory: factory,
        scopeKey: "review:transition:Review",
      });
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
    expect(result.current.lastSuccessfulSaveWasTopologyAffecting).toBe(false);
  });
});
