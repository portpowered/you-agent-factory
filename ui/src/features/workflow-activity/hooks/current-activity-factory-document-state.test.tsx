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
});
