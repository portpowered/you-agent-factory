import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { DashboardWorkstationRequest } from "../../api/dashboard/types";
import { ProviderSessionAttempts } from "./provider-session-attempts";
import {
  type LoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "./provider-session-details";
import { getWorkstationDetailMessages } from "./messages";

describe("ProviderSessionAttempts", () => {
  it("uses the default workstation-detail helper messages when no localized messages are provided", async () => {
    const user = userEvent.setup();
    const onSelectProviderSession = vi.fn();
    const onSelectWorkID = vi.fn();
    const onSelectWorkstationRequest = vi.fn();
    const expectedSession: LoadableProviderSessionRef = {
      dispatchID: "dispatch-review-active",
      id: "sess_active",
      kind: "session_id",
      provider: "codex",
    };
    const request: DashboardWorkstationRequest = {
      dispatch_id: "dispatch-review-active",
      dispatched_request_count: 1,
      errored_request_count: 0,
      inference_attempts: [],
      prompt: "Review the active story and decide whether it is ready.",
      responded_request_count: 1,
      transition_id: "transition-review",
      work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      workstation_name: "Review",
      workstation_node_id: "workstation-review",
    };

    render(
      <ProviderSessionAttempts
        attempts={[
          {
            dispatch_id: "dispatch-review-active",
            outcome: "ACCEPTED",
            provider_session: {
              id: expectedSession.id,
              kind: expectedSession.kind,
              provider: expectedSession.provider,
            },
            transition_id: "transition-review",
            workstation_name: "Review",
            work_items: [
              {
                display_name: "Active Story",
                trace_id: "trace-active-story",
                work_id: "work-active-story",
                work_type_id: "story",
              },
            ],
          },
          {
            dispatch_id: "dispatch-review-missing-details",
            outcome: "FAILED",
            provider_session: {
              id: "unsupported-session",
              kind: "path",
              provider: "codex",
            },
            transition_id: "transition-review",
            workstation_name: "Review",
          },
        ]}
        currentDispatchID="dispatch-review-active"
        emptyMessage="No workstation runs have been recorded for this workstation yet."
        onSelectProviderSession={onSelectProviderSession}
        onSelectWorkID={onSelectWorkID}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        renderHeading={(attempt) => attempt.dispatch_id}
        selectedProviderSessionKey={providerSessionSelectionKey(
          expectedSession,
        )}
        workstationKind="standard"
        workstationRequestsByDispatchID={{
          [request.dispatch_id]: request,
        }}
      />,
    );

    expect(screen.getByText("Current dispatch")).toBeTruthy();
    expect(screen.getByText("Current dispatch").className).toContain(
      "text-on-foreground",
    );
    expect(
      screen.getByRole("button", { name: "Select work item Active Story" }),
    ).toBeTruthy();
    expect(screen.getByText("Open Active Story")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: "Select workstation request dispatch-review-active",
      }),
    ).toBeTruthy();
    expect(screen.getByText("Open request details")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: "Select provider session codex / session_id / sess_active for dispatch dispatch-review-active",
      }),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", {
          name: "Select provider session codex / session_id / sess_active for dispatch dispatch-review-active",
        })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(screen.getByText("Session selected")).toBeTruthy();
    expect(
      screen.getByText("Session selected").closest("button")?.className,
    ).toContain("border-on-foreground");
    expect(
      screen.getByText("Session selected").closest("button")?.className,
    ).toContain("text-on-foreground");
    expect(screen.getByText("Session details unavailable")).toBeTruthy();
    expect(screen.getAllByText("Session log unavailable")).toHaveLength(2);
    expect(
      screen.getByText(
        "Work details unavailable for dispatch dispatch-review-missing-details.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Request details unavailable for dispatch dispatch-review-missing-details.",
      ),
    ).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Select work item Active Story" }),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Select workstation request dispatch-review-active",
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Select provider session codex / session_id / sess_active for dispatch dispatch-review-active",
      }),
    );
    screen
      .getByRole("button", {
        name: "Select provider session codex / session_id / sess_active for dispatch dispatch-review-active",
      })
      .focus();
    await user.keyboard("{Enter}");

    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story");
    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(request);
    expect(onSelectProviderSession).toHaveBeenNthCalledWith(1, expectedSession);
    expect(onSelectProviderSession).toHaveBeenNthCalledWith(2, expectedSession);
  });

  it("renders zh-CN provider-session selection actions and accessible names", () => {
    const localizedMessages = getWorkstationDetailMessages("zh-CN");

    render(
      <ProviderSessionAttempts
        attempts={[
          {
            dispatch_id: "dispatch-review-active",
            outcome: "ACCEPTED",
            provider_session: {
              id: "sess_active",
              kind: "session_id",
              provider: "codex",
            },
            transition_id: "transition-review",
            workstation_name: "Review",
            work_items: [
              {
                display_name: "Active Story",
                trace_id: "trace-active-story",
                work_id: "work-active-story",
                work_type_id: "story",
              },
            ],
          },
        ]}
        currentDispatchID="dispatch-review-active"
        emptyMessage="这个工作站暂时还没有记录任何运行。"
        messages={localizedMessages}
        onSelectProviderSession={vi.fn()}
        renderHeading={(attempt) => attempt.dispatch_id}
        selectedProviderSessionKey={null}
        workstationKind="standard"
      />,
    );

    expect(screen.getByText("当前分派")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: "选择调度 dispatch-review-active 的 provider session codex / session_id / sess_active",
      }),
    ).toBeTruthy();
    expect(screen.getByText("查看会话详情")).toBeTruthy();
  });
});
