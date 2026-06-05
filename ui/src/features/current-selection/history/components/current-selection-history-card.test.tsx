import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "./current-selection-history-card";

describe("CurrentSelectionHistoryCard", () => {
  it("renders history card chrome and optional highlighted tone", () => {
    render(
      <CurrentSelectionHistoryCard highlighted>
        History item
      </CurrentSelectionHistoryCard>,
    );

    const card = screen.getByText("History item");

    expect(card.tagName).toBe("ARTICLE");
    expect(card.className).toContain("grid");
    expect(card.className).toContain("border-primary");
  });
});

describe("CurrentSelectionHistoryCardHeader", () => {
  it("renders title, subtitle, badges, and identifier pill", () => {
    render(
      <CurrentSelectionHistoryCardHeader
        badges={<span>Current</span>}
        identifier="dispatch-1"
        subtitle="Pending"
        title="Review work"
      />,
    );

    expect(screen.getByText("Review work").tagName).toBe("STRONG");
    expect(screen.getByText("Pending").className).toContain(
      "text-on-surface-variant",
    );
    expect(screen.getByText("Current")).toBeTruthy();
    expect(screen.getByText("dispatch-1").className).toContain("rounded-full");
  });

  it("omits the metadata row when there is no subtitle or badge", () => {
    const { container } = render(
      <CurrentSelectionHistoryCardHeader title="Operator move" />,
    );

    expect(screen.getByText("Operator move")).toBeTruthy();
    expect(container.querySelector(".flex.flex-wrap")).toBeNull();
  });
});
