import { describe, expect, it } from "vitest";

import {
  mapGraphSaveOutcomeToDocumentSaveState,
  STALE_FACTORY_GRAPH_DRAFT_WARNING,
} from "../document-save/graph-document-save-state";

describe("mapGraphSaveOutcomeToDocumentSaveState", () => {
  it("maps submitting, error, warning, and idle outcomes", () => {
    expect(
      mapGraphSaveOutcomeToDocumentSaveState({
        errorMessage: null,
        isSubmitting: true,
        isStale: false,
      }),
    ).toEqual({ status: "submitting" });

    expect(
      mapGraphSaveOutcomeToDocumentSaveState({
        errorMessage: "API unavailable",
        isSubmitting: false,
        isStale: false,
      }),
    ).toEqual({
      errorMessage: "API unavailable",
      status: "error",
    });

    expect(
      mapGraphSaveOutcomeToDocumentSaveState({
        errorMessage: null,
        isSubmitting: false,
        isStale: true,
      }),
    ).toEqual({
      message: STALE_FACTORY_GRAPH_DRAFT_WARNING,
      status: "warning",
    });

    expect(
      mapGraphSaveOutcomeToDocumentSaveState({
        errorMessage: null,
        isSubmitting: false,
        isStale: false,
      }),
    ).toEqual({ status: "idle" });
  });
});
