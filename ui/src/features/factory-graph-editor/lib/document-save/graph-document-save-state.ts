import type { FactoryDocumentSaveState } from "../../../current-selection/base/hooks/factory-document-save-types";

export const STALE_FACTORY_GRAPH_DRAFT_WARNING =
  "The factory definition changed while you were editing. Refresh or discard your draft before saving.";

export function mapGraphSaveOutcomeToDocumentSaveState({
  errorMessage,
  isSubmitting,
  isStale,
}: {
  errorMessage: string | null;
  isSubmitting: boolean;
  isStale: boolean;
}): FactoryDocumentSaveState {
  if (isSubmitting) {
    return { status: "submitting" };
  }
  if (isStale) {
    return {
      message: STALE_FACTORY_GRAPH_DRAFT_WARNING,
      status: "warning",
    };
  }
  if (errorMessage) {
    return {
      errorMessage,
      status: "error",
    };
  }

  return { status: "idle" };
}
