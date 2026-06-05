import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { getCurrentSelectionDispatchHistoryMessages } from "../../base/messages/current-selection-dispatch-history";
import type { RelatedWorkItem } from "../lib/work-item-relationship-groups";
import {
  RelationshipLane,
  RelationshipLegend,
  RelationshipNodeCard,
} from "./work-item-relationship-map";

const messages = getCurrentSelectionDispatchHistoryMessages("en");

const relatedWork: RelatedWorkItem = {
  description: "Depends on while done",
  edgeLabel: "Depends on",
  group: "depends-on",
  key: "DEPENDS_ON:work-active:work-api:done",
  node: {
    label: "API contract",
    state: "blocked",
    traceID: "trace-api",
    workID: "work-api",
    workTypeID: "task",
  },
  workID: "work-api",
  workLabel: "API contract",
};

describe("work item relationship map components", () => {
  it("renders the relationship legend with semantic direction pills", () => {
    render(<RelationshipLegend messages={messages} />);

    const legend = screen.getByRole("region", {
      name: "Relationship key",
    });

    expect(
      within(legend).getByText("Parent work above the current selection"),
    ).toBeTruthy();
    expect(
      within(legend).getByText(
        "Dependent work blocked by the current selection",
      ),
    ).toBeTruthy();
  });

  it("renders relationship lanes and selects related work", () => {
    const onSelectWorkID = vi.fn();

    render(
      <RelationshipLane
        items={[relatedWork]}
        label={messages.relationshipDependsOnLabel}
        messages={messages}
        onSelectWorkID={onSelectWorkID}
      />,
    );

    const lane = screen.getByRole("region", {
      name: "Depends on relationships",
    });

    expect(within(lane).getByText("Depends on while done")).toBeTruthy();

    fireEvent.click(
      within(lane).getByRole("button", {
        name: "Select related work item API contract",
      }),
    );

    expect(onSelectWorkID).toHaveBeenCalledWith("work-api");
  });

  it("renders selected nodes as non-interactive selected surface cards", () => {
    render(
      <RelationshipNodeCard
        ariaCurrent="true"
        heading="Selected work"
        isSelected
        label="Active Story"
        messages={messages}
        node={{
          label: "Active Story",
          state: "running",
          workID: "work-active",
          workTypeID: "story",
        }}
      />,
    );

    expect(screen.getByText("Current selection")).toBeTruthy();
    expect(screen.getByText("Active Story").tagName).toBe("CODE");
    expect(screen.getByText("running")).toBeTruthy();
  });

  it("omits empty lanes", () => {
    const { container } = render(
      <RelationshipLane
        items={[]}
        label={messages.relationshipChildLabel}
        messages={messages}
      />,
    );

    expect(container.firstChild).toBeNull();
  });
});
