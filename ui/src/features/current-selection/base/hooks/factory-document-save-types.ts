export type FactoryDocumentSaveState<
  TFieldErrors extends Record<string, string> = Record<string, string>,
> =
  | { status: "idle" }
  | { status: "confirming" }
  | { status: "submitting" }
  | { status: "success" }
  | { message: string; status: "warning" }
  | {
      errorMessage: string;
      fieldErrors?: TFieldErrors;
      status: "error";
    };

export function isFactoryDocumentSaveSubmitting(
  saveState: FactoryDocumentSaveState,
): boolean {
  return saveState.status === "submitting";
}

export function isFactoryDocumentSaveSuccessful(
  saveState: FactoryDocumentSaveState,
): boolean {
  return saveState.status === "success";
}

export function getFactoryDocumentSaveErrorMessage(
  saveState: FactoryDocumentSaveState,
): string | null {
  return saveState.status === "error" ? saveState.errorMessage : null;
}
