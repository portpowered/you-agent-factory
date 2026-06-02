import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import type { DetailCardSaveState } from "../hooks/detail-card-save-types";
import { CURRENT_SELECTION_WARNING_PANEL_CLASS } from "./detail-card-shared";

export type DetailCardFactorySaveFeedbackMessages = {
  errorPrefix: string;
  staleVersionDetail: string;
  successMessage: string;
};

export type DetailCardFactorySaveFeedbackProps<
  TFieldErrors extends Record<string, string> = Record<string, string>,
> = {
  messages: DetailCardFactorySaveFeedbackMessages;
  saveState?: DetailCardSaveState<TFieldErrors>;
};

export function DetailCardFactorySaveFeedback<
  TFieldErrors extends Record<string, string> = Record<string, string>,
>({ messages, saveState }: DetailCardFactorySaveFeedbackProps<TFieldErrors>) {
  if (saveState?.status === "success") {
    return (
      <p
        className={cn("m-0 text-af-success-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="status"
      >
        {messages.successMessage}
      </p>
    );
  }

  if (saveState?.status === "warning") {
    return (
      <div className={CURRENT_SELECTION_WARNING_PANEL_CLASS}>
        <p
          className={cn("m-0 text-af-warning-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {saveState.message}
        </p>
        <p
          className={cn(
            "m-0 text-af-text-subtle",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.staleVersionDetail}
        </p>
      </div>
    );
  }

  if (saveState?.status === "error") {
    return (
      <p
        className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.errorPrefix} {saveState.errorMessage}
      </p>
    );
  }

  return null;
}

export function mergeDetailCardSaveFieldErrors<
  TValidationErrors extends Record<string, string | undefined>,
  TFieldErrors extends Record<string, string>,
>(
  validationErrors: TValidationErrors,
  saveState?: DetailCardSaveState<TFieldErrors>,
): TValidationErrors {
  if (saveState?.status !== "error" || !saveState.fieldErrors) {
    return validationErrors;
  }

  return {
    ...validationErrors,
    ...saveState.fieldErrors,
  };
}
