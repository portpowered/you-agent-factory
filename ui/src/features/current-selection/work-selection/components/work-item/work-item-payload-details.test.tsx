import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { WorkItemPayloadList } from "./work-item-payload-details";

const messages = {
  consumedPayloadEmpty: "No payload content",
  consumedPayloadError: "Payload failed to load",
  consumedPayloadHeading: "Consumed payload",
  consumedPayloadLoading: "Loading payload",
  consumedPayloadUnavailable: "Payload unavailable",
  consumedWorkItemsLabel: "Consumed work items",
  selectWorkItemLabel: (workItemLabel: string) =>
    `Select work item ${workItemLabel}`,
  stateLabel: "State",
  workTypeLabel: "Work type",
};

describe("WorkItemPayloadList", () => {
  it("renders plain consumed work items without nested surface styling and preserves selection behavior", () => {
    const onSelectWorkID = vi.fn();

    render(
      <WorkItemPayloadList
        messages={messages}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID="work-blocked-story"
        variant="plain"
        workItems={[
          {
            content: [{ text: "Dispatch-time consumed payload", type: "text" }],
            display_name: "Blocked Story",
            payload_status: "RESOLVED",
            state: "review",
            work_id: "work-blocked-story",
            work_type_id: "story",
          },
        ]}
      />,
    );

    const section = screen.getByText("Consumed work items").parentElement;
    const selectedWorkButton = screen.getByRole("button", {
      name: "Select work item Blocked Story",
    });

    expect(section).toBeTruthy();
    expect(selectedWorkButton.getAttribute("aria-pressed")).toBe("true");
    expect(selectedWorkButton.className).toContain("border-outline");
    expect(selectedWorkButton.className).not.toContain("bg-primary-container");
    expect(selectedWorkButton.className).not.toContain(
      "bg-secondary-container",
    );

    const plainArticle = selectedWorkButton.closest("article");

    expect(plainArticle?.className).toBe("grid gap-2");
    expect(plainArticle?.className).not.toContain("border-outline");
    expect(
      within(plainArticle as HTMLElement).getByText("Consumed payload"),
    ).toBeTruthy();
    expect(
      within(plainArticle as HTMLElement).getByText(
        "Dispatch-time consumed payload",
      ),
    ).toBeTruthy();

    fireEvent.click(selectedWorkButton);

    expect(onSelectWorkID).toHaveBeenCalledWith("work-blocked-story");
  });
});
