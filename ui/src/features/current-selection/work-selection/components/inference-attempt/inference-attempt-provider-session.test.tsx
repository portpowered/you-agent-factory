import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { inferenceAttempt } from "../../../base/components/detail-card/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "../../../base/components/presentation/current-selection-locale";
import { InferenceAttemptProviderSessionDetails } from "./inference-attempt-provider-session";

describe("InferenceAttemptProviderSessionDetails", () => {
  it("renders loadable provider sessions as selectable controls", () => {
    const onSelectProviderSession = vi.fn();

    render(
      <InferenceAttemptProviderSessionDetails
        attempt={inferenceAttempt("dispatch-review", {
          provider_session: {
            id: "sess-ready",
            kind: "session_id",
            provider: "codex",
          },
        })}
        onSelectProviderSession={onSelectProviderSession}
      />,
    );

    const button = screen.getByRole("button", {
      name: "Select provider session codex / Session ID / sess-ready for dispatch dispatch-review",
    });

    expect(screen.getByText("Inspect session details")).toBeTruthy();
    expect(screen.getByText("codex / Session ID / sess-ready")).toBeTruthy();

    fireEvent.click(button);

    expect(onSelectProviderSession).toHaveBeenCalledWith({
      dispatchID: "dispatch-review",
      id: "sess-ready",
      kind: "session_id",
      provider: "codex",
    });
  });

  it("marks the selected provider session", () => {
    render(
      <InferenceAttemptProviderSessionDetails
        attempt={inferenceAttempt("dispatch-review", {
          provider_session: {
            id: "sess-ready",
            kind: "session_id",
            provider: "codex",
          },
        })}
        onSelectProviderSession={vi.fn()}
        selectedProviderSessionKey="dispatch-review:codex:session_id:sess-ready"
      />,
    );

    const button = screen.getByRole("button", {
      name: "Select provider session codex / Session ID / sess-ready for dispatch dispatch-review",
    });

    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("Session selected")).toBeTruthy();
  });

  it("shows unavailable copy for provider sessions that cannot be loaded", () => {
    render(
      <InferenceAttemptProviderSessionDetails
        attempt={inferenceAttempt("dispatch-review", {
          provider_session: {
            id: "sess-path",
            kind: "path",
            provider: "codex",
          },
        })}
      />,
    );

    expect(screen.getByText("codex / Path / sess-path")).toBeTruthy();
    expect(screen.getByText("Session details unavailable")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("omits empty provider session details", () => {
    const { container } = render(
      <InferenceAttemptProviderSessionDetails
        attempt={inferenceAttempt("dispatch-review", {
          provider_session: undefined,
        })}
      />,
    );

    expect(container.firstChild).toBeNull();
  });

  it("localizes provider session labels and unavailable state", () => {
    render(
      <CurrentSelectionLocaleProvider locale="ja">
        <InferenceAttemptProviderSessionDetails
          attempt={inferenceAttempt("dispatch-review", {
            provider_session: {
              id: "sess-path",
              kind: "path",
              provider: "codex",
            },
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByText("codex / パス / sess-path")).toBeTruthy();
    expect(screen.getByText("セッション詳細は利用できません")).toBeTruthy();
  });
});
