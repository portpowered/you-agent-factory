import type { PromptTemplateContract } from "../../api/current-factory-prompt-template";

export interface PromptEditorDiagnostic {
  endOffset?: number;
  kind: string;
  message: string;
  path?: string;
  sourceText?: string;
  startOffset?: number;
}

export type PromptEditorAutocompleteState =
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | { message: string; status: "empty" }
  | { contract: PromptTemplateContract; status: "ready" };
