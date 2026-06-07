import { describe, expect, it } from "vitest";

import { formatLocalDateTime } from "../../../../components/ui/formatters";
import { inferenceAttempt } from "../../base/components/detail-card-test-helpers";
import { getCurrentSelectionDetailMessages } from "../../base/messages/current-selection-detail";
import { getInferenceAttemptTimingSummary } from "./inference-attempt-timing";

describe("getInferenceAttemptTimingSummary", () => {
  it("prefers elapsed duration over response time", () => {
    const detailMessages = getCurrentSelectionDetailMessages("en");

    expect(
      getInferenceAttemptTimingSummary(
        inferenceAttempt("dispatch-review", {
          duration_millis: 740,
          response_time: "2026-04-08T12:01:02Z",
        }),
        detailMessages,
        "en",
      ),
    ).toBe("Elapsed time: 740ms");
  });

  it("falls back to response time when duration is unavailable", () => {
    const detailMessages = getCurrentSelectionDetailMessages("en");
    const responseTime = "2026-04-08T12:01:02Z";

    expect(
      getInferenceAttemptTimingSummary(
        inferenceAttempt("dispatch-review", {
          duration_millis: undefined,
          response_time: responseTime,
        }),
        detailMessages,
        "en",
      ),
    ).toBe(
      `Response time: ${formatLocalDateTime(responseTime, "Unavailable", "en")}`,
    );
  });

  it("omits the summary when neither timing value is available", () => {
    expect(
      getInferenceAttemptTimingSummary(
        inferenceAttempt("dispatch-review", {
          duration_millis: undefined,
          response_time: undefined,
        }),
        getCurrentSelectionDetailMessages("en"),
        "en",
      ),
    ).toBeUndefined();
  });
});
