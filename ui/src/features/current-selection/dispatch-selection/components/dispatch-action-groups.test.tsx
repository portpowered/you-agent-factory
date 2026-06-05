import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  TraceActionGroup,
  WorkItemActionGroup,
} from "./dispatch-action-groups";

describe("dispatch action groups", () => {
  it("renders selectable work items with selected state and accessible labels", () => {
    const onSelectWorkID = vi.fn();

    render(
      <WorkItemActionGroup
        items={[
          {
            display_name: "Active Story",
            work_id: "work-active-story",
            work_type_id: "story",
          },
          {
            work_id: "work-review",
            work_type_id: "task",
          },
        ]}
        label="Consumed work"
        onSelectWorkID={onSelectWorkID}
        selectedWorkID="work-active-story"
        selectWorkItemAccessibleLabel={(label) => `Select ${label}`}
      />,
    );

    const selectedWork = screen.getByRole("button", {
      name: "Select Active Story",
    });

    expect(selectedWork.getAttribute("aria-pressed")).toBe("true");
    expect(selectedWork.className).toContain("border-primary");

    fireEvent.click(screen.getByRole("button", { name: "Select work-review" }));

    expect(onSelectWorkID).toHaveBeenCalledWith("work-review");
  });

  it("renders trace actions as real buttons and marks the active trace", () => {
    const onSelectTraceID = vi.fn();

    render(
      <TraceActionGroup
        activeTraceID="trace-active"
        label="Trace IDs"
        onSelectTraceID={onSelectTraceID}
        selectedTraceSuffix=" selected"
        traceIDs={["trace-active", "trace-retry"]}
      />,
    );

    const activeTrace = screen.getByRole("button", {
      name: "trace-active selected",
    });

    expect(activeTrace.getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "trace-retry" }));

    expect(onSelectTraceID).toHaveBeenCalledWith("trace-retry");
  });

  it("omits empty action groups", () => {
    const { container: workContainer } = render(
      <WorkItemActionGroup
        items={[]}
        label="Consumed work"
        selectedWorkID="work-1"
        selectWorkItemAccessibleLabel={(label) => label}
      />,
    );
    const { container: traceContainer } = render(
      <TraceActionGroup
        label="Trace IDs"
        selectedTraceSuffix=" selected"
        traceIDs={[]}
      />,
    );

    expect(workContainer.firstChild).toBeNull();
    expect(traceContainer.firstChild).toBeNull();
  });
});
