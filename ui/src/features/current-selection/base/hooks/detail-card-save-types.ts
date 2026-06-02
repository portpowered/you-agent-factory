export type { FactoryDocumentSaveState } from "./factory-document-save-types";

/** @deprecated Use `FactoryDocumentSaveState` for new factory document save surfaces. */
export type DetailCardSaveState<
  TFieldErrors extends Record<string, string> = Record<string, string>,
> =
  import("./factory-document-save-types").FactoryDocumentSaveState<TFieldErrors>;
