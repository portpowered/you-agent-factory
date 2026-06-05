import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import { inferenceAttempt } from "../../base/components/detail-card-test-helpers";
import { InferenceAttemptSummaryHeader } from "./inference-attempt-summary-header";

describe("InferenceAttemptSummaryHeader", () => {
  it("renders collapsed attempt summary with timing and outcome", () => {
    render(
      <InferenceAttemptSummaryHeader
        attempt={inferenceAttempt("dispatch-review", {
          attempt: 2,
          duration_millis: 740,
          outcome: "COMPLETED",
        })}
        expanded={false}
        headingId="attempt-heading"
        onToggle={vi.fn()}
        panelId="attempt-panel"
        timingSummary="Elapsed: 740ms"
      />,
    );

    const trigger = screen.getByRole("button", { name: "Expand attempt 2" });

    expect(screen.getByText("Attempt 2").getAttribute("id")).toBe(
      "attempt-heading",
    );
    expect(screen.getByText("Unknown outcome: COMPLETED")).toBeTruthy();
    expect(screen.getByText("Elapsed: 740ms").className).toContain(
      "text-on-surface-subtle",
    );
    expect(trigger.getAttribute("aria-controls")).toBe("attempt-panel");
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  it("renders expanded action state and toggles through the supplied callback", () => {
    const onToggle = vi.fn();

    render(
      <InferenceAttemptSummaryHeader
        attempt={inferenceAttempt("dispatch-review", { attempt: 1 })}
        expanded
        headingId="attempt-heading"
        onToggle={onToggle}
        panelId="attempt-panel"
      />,
    );

    const trigger = screen.getByRole("button", { name: "Collapse attempt 1" });

    expect(trigger.getAttribute("aria-expanded")).toBe("true");

    fireEvent.click(trigger);

    expect(onToggle).toHaveBeenCalledTimes(1);
  });

  it("localizes attempt actions and unknown outcomes through the provider", () => {
    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <InferenceAttemptSummaryHeader
          attempt={inferenceAttempt("dispatch-review", {
            attempt: 1,
            outcome: "BLOCKED_FOR_REVIEW",
          })}
          expanded={false}
          headingId="attempt-heading"
          onToggle={vi.fn()}
          panelId="attempt-panel"
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByRole("button", { name: "展开尝试 1" })).toBeTruthy();
    expect(screen.getByText("未知结果：BLOCKED_FOR_REVIEW")).toBeTruthy();
  });
});
