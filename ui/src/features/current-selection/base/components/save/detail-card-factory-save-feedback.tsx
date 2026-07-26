import { AlertPanel, AlertPanelText } from "@you-agent-factory/components/feedback";
import type { DetailCardSaveState } from "../../hooks/detail-card-save-types";

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
        <AlertPanelText>{messages.successMessage}</AlertPanelText>
      </AlertPanel>
    );
  }

  if (saveState?.status === "warning") {
    return (
      <AlertPanel role="alert" tone="warning">
        <AlertPanelText>{saveState.message}</AlertPanelText>
        <AlertPanelText className="text-on-surface-subtle" variant="supporting">
          {messages.staleVersionDetail}
        </AlertPanelText>
      </AlertPanel>
    );
  }

  if (saveState?.status === "error") {
    return (
      <AlertPanel role="alert" tone="danger">
        <AlertPanelText>
          {messages.errorPrefix} {saveState.errorMessage}
        </AlertPanelText>
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
