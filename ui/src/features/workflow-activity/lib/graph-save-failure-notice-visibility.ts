export function shouldShowGraphSaveFailureNotice({
  dismissedSaveFailureRevision,
  hasFailureMessages,
  saveAttemptRevision,
}: {
  dismissedSaveFailureRevision: number | null;
  hasFailureMessages: boolean;
  saveAttemptRevision: number;
}): boolean {
  if (!hasFailureMessages) {
    return false;
  }

  return dismissedSaveFailureRevision !== saveAttemptRevision;
}
