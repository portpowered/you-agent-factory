export * from "../state/selection-types";
export * from "../state/dashboardSelection";
export * from "../state/dashboardStatePlaces";
export * from "../state/selectionHistoryStore";

export * from "../messages/current-selection-detail";
export * from "../messages/current-selection-shell";
export * from "../messages/current-selection-operational-enums";
export * from "../messages/current-selection-dispatch-history";

export * from "../components/current-selection-locale";
export * from "../components/current-selection-detail-layout";
export * from "../components/current-selection-header-actions";
export * from "../components/detail-card-shared";
export * from "../components/detail-card-types";
export {
  EditableConfigurationSaveRow,
  type EditableConfigurationSaveRowProps,
} from "../components/editable-configuration-save-row";
export {
  DetailCardFactorySaveFeedback,
  mergeDetailCardSaveFieldErrors,
  type DetailCardFactorySaveFeedbackMessages,
  type DetailCardFactorySaveFeedbackProps,
} from "../components/detail-card-factory-save-feedback";
export { NoSelectionDetailCard } from "../components/no-selection-detail-card";

export * from "../hooks/detail-card-save-types";
export type { FactoryDocumentSaveState } from "../hooks/factory-document-save-types";
export {
  useFactoryDocumentSave,
  type FactoryDocumentSaveInput,
} from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
export {
  useScopedFactoryDocumentSave,
  type ScopedFactoryDocumentSaveRequest,
  type UseScopedFactoryDocumentSaveOptions,
  type UseScopedFactoryDocumentSaveResult,
} from "../hooks/useScopedFactoryDocumentSave";
