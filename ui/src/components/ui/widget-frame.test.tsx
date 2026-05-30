import { render, screen, within } from "@testing-library/react";

import { DASHBOARD_PANEL_SHELL_CLASS } from "./dashboard-shell";
import {
  DashboardWidgetFrame,
  DETAIL_CARD_CLASS,
  DETAIL_COPY_CLASS,
  EMPTY_STATE_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "./widget-frame";

describe("DashboardWidgetFrame", () => {
  it("renders the shared widget frame contract with dashboard copy styles intact", () => {
    render(
      <DashboardWidgetFrame title="Submit work" widgetId="submit-work">
        <p className={WIDGET_SUBTITLE_CLASS}>Queue a new request</p>
        <p className={DETAIL_COPY_CLASS}>
          Submissions stay inside the shared layout frame.
        </p>
        <div className={EMPTY_STATE_CLASS}>
          <h3>No active submission</h3>
        </div>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Submit work" });
    const title = within(card).getByRole("heading", {
      level: 3,
      name: "Submit work",
    });
    const header = card.querySelector("header");

    expect(title).toBeTruthy();
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(header?.className).toContain("cursor-grab");
    expect(
      within(card).queryByRole("button", { name: "Move Submit work" }),
    ).toBeNull();
    expect(card.querySelectorAll("header")).toHaveLength(1);

    const subtitle = within(card).getByText("Queue a new request");
    const bodyCopy = within(card).getByText(
      "Submissions stay inside the shared layout frame.",
    );
    const emptyHeading = within(card).getByRole("heading", {
      name: "No active submission",
    });

    expect(card.className).toContain("min-w-0");
    expect(card.className).toContain(DASHBOARD_PANEL_SHELL_CLASS);
    expect(card.className).toContain(DETAIL_CARD_CLASS);
    expect(card.className).toContain("border-af-border");
    expect(card.className).toContain("bg-af-surface-raised");
    expect(subtitle.className).toContain(WIDGET_SUBTITLE_CLASS);
    expect(bodyCopy.className).toContain(DETAIL_COPY_CLASS);
    expect(emptyHeading.parentElement?.className).toContain(EMPTY_STATE_CLASS);
    expect(emptyHeading.parentElement?.className).toContain("border-af-border-strong");
    expect(emptyHeading.parentElement?.className).toContain("bg-af-surface-subtle");
  });

  it("routes header actions through AgentBentoCard without a custom header slot", () => {
    render(
      <DashboardWidgetFrame
        headerAction={<button type="button">Remove card</button>}
        title="Provider session"
        widgetId="provider-session"
      >
        <p>Session details</p>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Provider session" });
    const header = card.querySelector("header");
    const toolsRegion = header?.lastElementChild;

    expect(
      within(card).getByRole("heading", { level: 3, name: "Provider session" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("button", { name: "Remove card" }),
    ).toBeTruthy();
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(header?.className).toContain("cursor-grab");
    expect(
      within(card).queryByRole("button", { name: "Move Provider session" }),
    ).toBeNull();
    expect(toolsRegion?.contains(screen.getByRole("button", { name: "Remove card" }))).toBe(
      true,
    );
  });
});
