import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { useCurrentActivityFactoryDocumentState } from "./current-activity-factory-document-state";

describe("useCurrentActivityFactoryDocumentState", () => {
  it("keeps a versioned legacy event observer-only until activation provenance arrives", () => {
    const factory = structuredClone(semanticWorkflowDashboardSnapshot.factory);
    if (factory == null) {
      throw new Error("dashboard fixture must include a Factory");
    }

    const { activation: _activation, ...factoryWithoutActivation } = factory;
    const legacyFactory: NonNullable<DashboardSnapshot["factory"]> = {
      ...factoryWithoutActivation,
      version: {
        logical: "7",
        physical: "2026-09-13T12:00:00Z",
      },
    };

    const { result } = renderHook(() =>
      useCurrentActivityFactoryDocumentState({ eventFactory: legacyFactory }),
    );

    expect(result.current.currentFactoryDocument).toBeUndefined();
    expect(result.current.editableDefinitionQuery).toMatchObject({
      data: undefined,
      error: null,
      status: "success",
    });
  });

  it("projects activation-bearing events into an editable current document", () => {
    const factory = structuredClone(semanticWorkflowDashboardSnapshot.factory);
    if (factory == null) {
      throw new Error("dashboard fixture must include a Factory");
    }

    const authoritativeFactory: NonNullable<DashboardSnapshot["factory"]> = {
      ...factory,
      activation: {
        activationId: "authoritative-activation",
        loadedSourceDigest: `sha256:${"a".repeat(64)}`,
        state: "ACTIVE",
      },
      version: {
        logical: "8",
        physical: "2026-09-13T12:01:00Z",
      },
    };

    const { result } = renderHook(() =>
      useCurrentActivityFactoryDocumentState({
        eventFactory: authoritativeFactory,
      }),
    );

    expect(result.current.currentFactoryDocument).toMatchObject({
      activation: authoritativeFactory.activation,
      version: authoritativeFactory.version,
    });
    expect(result.current.editableDefinitionQuery).toMatchObject({
      data: expect.objectContaining({
        activation: authoritativeFactory.activation,
      }),
      error: null,
      status: "success",
    });
  });

  it("enables the editable document when a legacy event is replaced by authoritative data", () => {
    const factory = structuredClone(semanticWorkflowDashboardSnapshot.factory);
    if (factory == null) {
      throw new Error("dashboard fixture must include a Factory");
    }

    const { activation: _activation, ...factoryWithoutActivation } = factory;
    const legacyFactory: NonNullable<DashboardSnapshot["factory"]> = {
      ...factoryWithoutActivation,
      version: {
        logical: "7",
        physical: "2026-09-13T12:00:00Z",
      },
    };
    const authoritativeFactory: NonNullable<DashboardSnapshot["factory"]> = {
      ...legacyFactory,
      activation: {
        activationId: "authoritative-activation",
        loadedSourceDigest: `sha256:${"b".repeat(64)}`,
        state: "ACTIVE",
      },
    };

    const { rerender, result } = renderHook(
      ({ eventFactory }) =>
        useCurrentActivityFactoryDocumentState({ eventFactory }),
      { initialProps: { eventFactory: legacyFactory } },
    );

    expect(result.current.currentFactoryDocument).toBeUndefined();
    expect(result.current.editableDefinitionQuery.status).toBe("success");

    rerender({ eventFactory: authoritativeFactory });

    expect(result.current.currentFactoryDocument?.activation).toEqual(
      authoritativeFactory.activation,
    );
    expect(result.current.editableDefinitionQuery).toMatchObject({
      data: expect.objectContaining({
        activation: authoritativeFactory.activation,
      }),
      status: "success",
    });
  });
});
