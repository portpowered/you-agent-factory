import { AlertPanel, DashboardText } from "../../../../components/ui";
import type { DetailCardSaveState } from "../hooks/detail-card-save-types";

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
      <AlertPanel role="status" tone="success">
        <DashboardText className="m-0">
          {messages.successMessage}
        </DashboardText>
      </AlertPanel>
    );
  }

  if (saveState?.status === "warning") {
    return (
      <AlertPanel role="alert" tone="warning">
        <DashboardText className="m-0">
          {saveState.message}
        </DashboardText>
        <DashboardText
          className="m-0 text-on-surface-subtle"
          variant="supporting"
        >
          {messages.staleVersionDetail}
        </DashboardText>
      </AlertPanel>
    );
  }

  if (saveState?.status === "error") {
    return (
      <AlertPanel role="alert" tone="danger">
        <DashboardText className="m-0">
          {messages.errorPrefix} {saveState.errorMessage}
        </DashboardText>
      </AlertPanel>
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
