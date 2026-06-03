import { describe, expect, it } from "vitest";

import { shouldShowGraphSaveFailureNotice } from "./graph-save-failure-notice-visibility";

describe("shouldShowGraphSaveFailureNotice", () => {
  it("hides the notice when there are no failure messages", () => {
    expect(
      shouldShowGraphSaveFailureNotice({
        dismissedSaveFailureRevision: null,
        hasFailureMessages: false,
        saveAttemptRevision: 1,
      }),
    ).toBe(false);
  });

  it("shows the notice for a new save failure revision", () => {
    expect(
      shouldShowGraphSaveFailureNotice({
        dismissedSaveFailureRevision: 1,
        hasFailureMessages: true,
        saveAttemptRevision: 2,
      }),
    ).toBe(true);
  });

  it("hides the notice after dismissal for the current revision", () => {
    expect(
      shouldShowGraphSaveFailureNotice({
        dismissedSaveFailureRevision: 2,
        hasFailureMessages: true,
        saveAttemptRevision: 2,
      }),
    ).toBe(false);
  });
});
