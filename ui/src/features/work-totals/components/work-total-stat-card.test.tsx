import { render, screen } from "@testing-library/react";

import { WorkTotalStatCard } from "./work-total-stat-card";

describe("WorkTotalStatCard", () => {
  it("renders semantic alert-backed tones for totals", () => {
    render(
      <>
        <WorkTotalStatCard
          label="In progress"
          tone="live"
          value={2}
          valueLabel="In progress: 2"
        />
        <WorkTotalStatCard
          label="Completed"
          tone="success"
          value={3}
          valueLabel="Completed: 3"
        />
        <WorkTotalStatCard
          label="Failed"
          tone="danger"
          value={1}
          valueLabel="Failed: 1"
        />
        <WorkTotalStatCard
          label="Dispatched"
          tone="neutral"
          value={5}
          valueLabel="Dispatched: 5"
        />
      </>,
    );

    expect(screen.getByLabelText("In progress: 2").className).toContain(
      "bg-info-container",
    );
    expect(screen.getByLabelText("Completed: 3").className).toContain(
      "bg-success-container",
    );
    expect(screen.getByLabelText("Failed: 1").className).toContain(
      "bg-error-container",
    );
    expect(screen.getByLabelText("Dispatched: 5").className).toContain(
      "bg-surface-container-low",
    );
  });

  it("formats localized stat values", () => {
    render(
      <WorkTotalStatCard
        label="已完成"
        locale="zh-CN"
        tone="success"
        value={12345}
        valueLabel="已完成：12,345"
      />,
    );

    expect(screen.getByText("12,345")).toBeTruthy();
  });
});
