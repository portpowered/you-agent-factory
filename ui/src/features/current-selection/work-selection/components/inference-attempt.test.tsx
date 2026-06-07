import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import { inferenceAttempt } from "../../base/components/detail-card-test-helpers";
import { InferenceAttemptCard } from "./inference-attempt";

describe("InferenceAttemptCard", () => {
  it("renders a collapsed preview with outcome, timing, and provider-session action", () => {
    const onSelectProviderSession = vi.fn();

    render(
      <CurrentSelectionLocaleProvider>
        <InferenceAttemptCard
          attempt={inferenceAttempt("dispatch-review", {
            attempt: 2,
            duration_millis: 740,
            outcome: "COMPLETED",
            provider_session: {
              id: "sess-ready",
              kind: "session_id",
              provider: "codex",
            },
          })}
          onSelectProviderSession={onSelectProviderSession}
        />
      </CurrentSelectionLocaleProvider>,
    );

    const article = screen.getByRole("article", {
      name: "Inference attempt 2",
    });

    expect(within(article).getByRole("heading", { name: "Attempt 2" })).toBeTruthy();
    expect(within(article).getByText("Unknown outcome: COMPLETED")).toBeTruthy();
    expect(within(article).getByText("Elapsed time: 740ms")).toBeTruthy();
    expect(
      within(article).getByRole("button", { name: /Select provider session .*sess-ready/i }),
    ).toBeTruthy();
    expect(within(article).queryByText("dispatch-review")).toBeNull();
  });

  it("reveals full details when expanded", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <InferenceAttemptCard
          attempt={inferenceAttempt("dispatch-review", {
            attempt: 1,
            inference_request_id: "dispatch-review/inference-request/full",
            duration_millis: 875,
            outcome: "FAILED",
            provider_session: {
              id: "sess-ready",
              kind: "session_id",
              provider: "codex-session",
            },
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    const article = screen.getByRole("article", {
      name: "Inference attempt 1",
    });
    const section = within(article)
      .getByRole("heading", { name: "Attempt 1" })
      .closest("section");

    if (!section) {
      throw new Error("expected attempt section");
    }

    fireEvent.click(within(section).getByRole("button", { name: "Expand attempt 1" }));

    expect(within(article).getByText("dispatch-review/inference-request/full")).toBeTruthy();
    expect(within(article).getByText("Provider session")).toBeTruthy();
  });
});
