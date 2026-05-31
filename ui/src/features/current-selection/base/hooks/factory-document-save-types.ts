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

export function isFactoryDocumentSaveConfirming(
  saveState: FactoryDocumentSaveState,
): boolean {
  return saveState.status === "confirming";
}

export function isFactoryDocumentSaveSubmitting(
  saveState: FactoryDocumentSaveState,
): boolean {
  return saveState.status === "submitting";
}
