export { MonacoPromptEditor } from "./monaco-prompt-editor";
export {
  buildWorkstationPromptCompletionItems,
  buildWorkstationPromptMarkers,
  extractTemplateExpression,
  getCurrentTemplateExpression,
  isInsideTemplate,
  registerWorkstationPromptCompletionProvider,
  registerWorkstationPromptMonaco,
  resetWorkstationPromptMonacoRegistrationForTests,
  WORKSTATION_PROMPT_LANGUAGE_ID,
  WORKSTATION_PROMPT_THEME_ID,
} from "./monaco-prompt-setup";
export type {
  PromptEditorAutocompleteState,
  PromptEditorDiagnostic,
} from "./prompt-editor-types";
