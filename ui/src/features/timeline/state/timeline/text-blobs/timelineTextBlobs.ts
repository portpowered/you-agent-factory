import type { DashboardInferenceAttempt } from "../../../../../api/dashboard";
import type { ReplayTextBacked, ReplayTextBlobState } from "../types";

export function storeTextBlob(
  state: ReplayTextBlobState,
  textBlobID: string,
  value: string | undefined,
): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  state.textBlobsByID ??= {};
  state.textBlobsByID[textBlobID] = value;
  return textBlobID;
}

export function resolveTextBlob(
  textBlobsByID: Record<string, string> | undefined,
  textBlobID: string | undefined,
  fallback = "",
): string {
  return textBlobID ? (textBlobsByID?.[textBlobID] ?? fallback) : fallback;
}

export function hydrateInferenceAttempt(
  attempt: DashboardInferenceAttempt & ReplayTextBacked,
  textBlobsByID: Record<string, string> | undefined,
): DashboardInferenceAttempt {
  return {
    ...attempt,
    prompt: resolveTextBlob(
      textBlobsByID,
      attempt.promptTextBlobID,
      attempt.prompt,
    ),
    response: attempt.responseTextBlobID
      ? resolveTextBlob(textBlobsByID, attempt.responseTextBlobID)
      : attempt.response,
  };
}

export function hydrateInferenceAttemptsByDispatchID(
  attemptsByDispatchID: Record<
    string,
    Record<string, DashboardInferenceAttempt & ReplayTextBacked>
  >,
  textBlobsByID: Record<string, string> | undefined,
): Record<string, Record<string, DashboardInferenceAttempt>> {
  return Object.fromEntries(
    Object.entries(attemptsByDispatchID).map(([dispatchID, attempts]) => [
      dispatchID,
      Object.fromEntries(
        Object.entries(attempts).map(([requestID, attempt]) => [
          requestID,
          hydrateInferenceAttempt(attempt, textBlobsByID),
        ]),
      ),
    ]),
  );
}
