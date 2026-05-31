export {
  useFactoryDocumentSave,
  type FactoryDocumentSaveInput,
} from "../hooks/useFactoryDocumentSave";

export {
  type FactoryDocumentSaveState,
  getFactoryDocumentSaveErrorMessage,
  isFactoryDocumentSaveConfirming,
  isFactoryDocumentSaveSubmitting,
  isFactoryDocumentSaveSuccessful,
} from "../../current-selection/base/hooks/factory-document-save-types";

export {
  useScopedFactoryDocumentSave,
  type ScopedFactoryDocumentSaveRequest,
  type UseScopedFactoryDocumentSaveOptions,
  type UseScopedFactoryDocumentSaveResult,
} from "../../current-selection/base/hooks/useScopedFactoryDocumentSave";
