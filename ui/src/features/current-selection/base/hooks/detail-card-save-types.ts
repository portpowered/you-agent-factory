export type DetailCardSaveState<
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
