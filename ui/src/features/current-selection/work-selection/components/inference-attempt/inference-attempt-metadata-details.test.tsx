import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { formatLocalDateTime } from "../../../../../components/ui/formatters";
import { inferenceAttempt } from "../../../base/components/detail-card/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "../../../base/components/presentation/current-selection-locale";
import { InferenceAttemptMetadataDetails } from "./inference-attempt-metadata-details";

describe("InferenceAttemptMetadataDetails", () => {
  it("renders inference attempt metadata details", () => {
    const requestTime = "2026-04-08T12:00:01Z";
    const responseTime = "2026-04-08T12:01:02Z";

    render(
      <CurrentSelectionLocaleProvider locale="en">
        <InferenceAttemptMetadataDetails
          attempt={inferenceAttempt("dispatch-review", {
            diagnostics: {
              provider: {
                model: "gpt-5.4-review",
                provider: "codex-diagnostics",
              },
            },
            duration_millis: 875,
            error_class: "ProviderTimeout",
            exit_code: 12,
            inference_request_id: "dispatch-review/inference-request/full",
            outcome: "FAILED",
            provider_session: {
              id: "sess-ready",
              kind: "session_id",
              provider: "codex-session",
            },
            request_time: requestTime,
            response_time: responseTime,
            working_directory: "/repo/infinite-you",
            worktree: "/repo/infinite-you/.worktrees/review",
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(
      screen.getByText("dispatch-review/inference-request/full"),
    ).toBeTruthy();
    expect(screen.getByText("codex-diagnostics")).toBeTruthy();
    expect(screen.getByText("gpt-5.4-review")).toBeTruthy();
    expect(screen.queryByText("codex-session")).toBeNull();
    expect(screen.getByText("/repo/infinite-you")).toBeTruthy();
    expect(
      screen.getByText("/repo/infinite-you/.worktrees/review"),
    ).toBeTruthy();
    expect(
      screen.getByText(formatLocalDateTime(requestTime, "Unavailable", "en")),
    ).toBeTruthy();
    expect(
      screen.getByText(formatLocalDateTime(responseTime, "Unavailable", "en")),
    ).toBeTruthy();
    expect(screen.getByTitle(requestTime)).toBeTruthy();
    expect(screen.getByTitle(responseTime)).toBeTruthy();
    expect(screen.getByText("Failed")).toBeTruthy();
    expect(screen.getByText("875ms")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("ProviderTimeout")).toBeTruthy();
  });

  it("falls back to provider session provider when diagnostics provider is unavailable", () => {
    render(
      <InferenceAttemptMetadataDetails
        attempt={inferenceAttempt("dispatch-review", {
          diagnostics: undefined,
          provider_session: {
            id: "sess-ready",
            kind: "session_id",
            provider: "codex-session",
          },
        })}
      />,
    );

    expect(screen.getByText("codex-session")).toBeTruthy();
  });

  it("localizes unavailable timestamps", () => {
    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <InferenceAttemptMetadataDetails
          attempt={inferenceAttempt("dispatch-review", {
            request_time: undefined,
            response_time: "not-a-date",
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getAllByText("不可用")).toHaveLength(2);
    expect(screen.queryByText("not-a-date")).toBeNull();
  });
});

describe("InferenceAttemptMetadataDetails failure details", () => {
  it("renders the canonical Codex failure and clears it for a successful attempt", () => {
    const { rerender } = render(
      <CurrentSelectionLocaleProvider locale="en">
        <InferenceAttemptMetadataDetails
          attempt={inferenceAttempt("dispatch-codex", {
            failure_detail: {
              reason: "permanent_bad_request",
              message:
                "Model gpt-5.6-sol requires a newer version of Codex. Please update Codex and retry.",
            },
            outcome: "FAILED",
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByText("Failure reason")).toBeTruthy();
    expect(screen.getByText("permanent_bad_request")).toBeTruthy();
    expect(
      screen.getByText(
        "Model gpt-5.6-sol requires a newer version of Codex. Please update Codex and retry.",
      ),
    ).toBeTruthy();

    rerender(
      <CurrentSelectionLocaleProvider locale="en">
        <InferenceAttemptMetadataDetails
          attempt={inferenceAttempt("dispatch-success", {
            failure_detail: undefined,
            outcome: "SUCCEEDED",
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.queryByText("Failure reason")).toBeNull();
    expect(screen.queryByText("permanent_bad_request")).toBeNull();
  });

  it("localizes a translated historical failure with no message", () => {
    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <InferenceAttemptMetadataDetails
          attempt={inferenceAttempt("dispatch-history", {
            failure_detail: { reason: "unknown" },
            outcome: "FAILED",
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByText("unknown")).toBeTruthy();
    expect(screen.getByText("失败消息")).toBeTruthy();
    expect(screen.getByText("此请求没有可用的失败消息。")).toBeTruthy();
  });
});
