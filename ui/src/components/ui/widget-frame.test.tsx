import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
  WIDGET_FRAME_SUBTITLE_CLASS,
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetErrorState,
  WidgetFrame,
  WidgetFrameDisclosure,
  WidgetFrameDisclosurePanel,
  WidgetFrameDisclosureTrigger,
  WidgetLoadingState,
  WidgetSubtitle,
  WidgetSuccessState,
  widgetFrameHasNoHorizontalOverflow,
  widgetFrameStoryShellStyle,
} from "@you-agent-factory/components/recipes";
import { vi } from "vitest";

describe("WidgetEmptyState", () => {
  it("renders compact dashboard empty states through the component contract", () => {
    render(
      <WidgetEmptyState compact>
        <h3>No trace selected</h3>
      </WidgetEmptyState>,
    );

    const emptyHeading = screen.getByRole("heading", {
      name: "No trace selected",
    });
    expect(emptyHeading.parentElement?.className).toContain("min-h-0");
    expect(emptyHeading.parentElement?.className).toContain(
      "bg-surface-container-low",
    );
  });

  it("renders empty-state title and body copy through shared typography roles", () => {
    render(
      <WidgetEmptyState>
        <WidgetEmptyStateTitle as="h2">No chart data</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Run the factory to populate this trend.
        </WidgetEmptyStateText>
      </WidgetEmptyState>,
    );

    const title = screen.getByRole("heading", {
      level: 2,
      name: "No chart data",
    });
    const body = screen.getByText("Run the factory to populate this trend.");

    expect(title.className).toContain(WIDGET_FRAME_SECTION_HEADING_CLASS);
    expect(body.className).toContain(WIDGET_FRAME_BODY_TEXT_CLASS);
    expect(body.className).toContain("m-0");
  });
});

describe("WidgetSubtitle", () => {
  it("supports subtitle text on non-paragraph semantic elements", () => {
    render(
      <dl>
        <WidgetSubtitle as="dd">42 completed</WidgetSubtitle>
      </dl>,
    );

    const value = screen.getByText("42 completed");

    expect(value.tagName).toBe("DD");
    expect(value.className).toContain(WIDGET_FRAME_SUBTITLE_CLASS);
  });
});

describe("WidgetFrame", () => {
  it("renders the package widget frame shell with host-provided content", () => {
    render(
      <WidgetFrame title="Example widget">
        <WidgetSubtitle>Queue a new request</WidgetSubtitle>
        <WidgetDetailCopy>
          Submissions stay inside the shared layout frame.
        </WidgetDetailCopy>
        <WidgetEmptyState>
          <h3>No active submission</h3>
        </WidgetEmptyState>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Example widget" });
    const title = within(card).getByRole("heading", {
      level: 3,
      name: "Example widget",
    });

    expect(title).toBeTruthy();
    expect(card.querySelectorAll("header")).toHaveLength(1);
    expect(card.className).toContain("min-w-0");
    expect(card.className).toContain("shadow-af-card");
    expect(card.className).toContain("[&_dl]:grid");
    expect(card.className).toContain("border-outline");
    expect(card.className).toContain("bg-surface-container-high");

    const subtitle = within(card).getByText("Queue a new request");
    const bodyCopy = within(card).getByText(
      "Submissions stay inside the shared layout frame.",
    );
    const emptyHeading = within(card).getByRole("heading", {
      name: "No active submission",
    });

    expect(subtitle.className).toContain(WIDGET_FRAME_SUBTITLE_CLASS);
    expect(bodyCopy.className).toContain(WIDGET_FRAME_BODY_TEXT_CLASS);
    expect(emptyHeading.parentElement?.className).toContain("border-dashed");
  });

  it("supports the wide widget frame layout variant", () => {
    render(
      <WidgetFrame title="Trend card" wide>
        <p>Trend content</p>
      </WidgetFrame>,
    );

    expect(
      screen.getByRole("article", { name: "Trend card" }).className,
    ).toContain("min-h-72");
  });

  it("passes body props through to the widget frame body", () => {
    render(
      <WidgetFrame
        bodyClassName="custom-widget-body"
        bodyProps={{ "data-widget-body": "trace-card" }}
        title="Trace"
      >
        <p>Trace content</p>
      </WidgetFrame>,
    );

    const body = screen
      .getByText("Trace content")
      .closest("[data-widget-body]");

    expect(body?.getAttribute("data-widget-body")).toBe("trace-card");
    expect(body?.className).toContain("custom-widget-body");
  });

  it("uses scrollable bodies by default", () => {
    render(
      <WidgetFrame title="Trace">
        <p>Trace content</p>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Trace" });
    const body = screen.getByText("Trace content").parentElement;

    expect(card.className).toContain("overflow-hidden");
    expect(body?.className).toContain("overflow-auto");
  });

  it("supports opting out of the internal scroll body", () => {
    render(
      <WidgetFrame bodyScroll={false} title="Trace">
        <p>Trace content</p>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Trace" });
    const body = screen.getByText("Trace content").parentElement;

    expect(card.className).not.toContain("overflow-hidden");
    expect(body?.className).not.toContain("overflow-auto");
  });

  it("routes header actions through the widget frame header", () => {
    render(
      <WidgetFrame
        headerAction={<button type="button">Remove card</button>}
        title="Provider session"
      >
        <p>Session details</p>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Provider session" });
    const header = card.querySelector("header");
    const toolsRegion = header?.lastElementChild;

    expect(
      within(card).getByRole("heading", {
        level: 3,
        name: "Provider session",
      }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("button", { name: "Remove card" }),
    ).toBeTruthy();
    expect(
      toolsRegion?.contains(
        screen.getByRole("button", { name: "Remove card" }),
      ),
    ).toBe(true);
  });
});

describe("Widget frame state panels", () => {
  it("renders host-provided loading copy with busy status semantics", () => {
    render(
      <WidgetLoadingState>
        <WidgetEmptyStateTitle>Loading content</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided loading message.
        </WidgetEmptyStateText>
      </WidgetLoadingState>,
    );

    const status = screen.getByRole("status");

    expect(status.getAttribute("aria-busy")).toBe("true");
    expect(
      screen.getByRole("heading", { name: "Loading content" }),
    ).toBeTruthy();
    expect(screen.getByText("Host-provided loading message.")).toBeTruthy();
    expect(status.querySelectorAll('[aria-hidden="true"]')).toHaveLength(4);
  });

  it("renders host-provided error copy with alert semantics", () => {
    render(
      <WidgetErrorState>
        <WidgetEmptyStateTitle>Request failed</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided error message.
        </WidgetEmptyStateText>
      </WidgetErrorState>,
    );

    const alert = screen.getByRole("alert");

    expect(alert.className).toContain("bg-error-container");
    expect(
      screen.getByRole("heading", { name: "Request failed" }),
    ).toBeTruthy();
    expect(screen.getByText("Host-provided error message.")).toBeTruthy();
  });

  it("renders host-provided success copy with status semantics", () => {
    render(
      <WidgetSuccessState>
        <WidgetEmptyStateTitle>Action completed</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided success message.
        </WidgetEmptyStateText>
      </WidgetSuccessState>,
    );

    const status = screen.getByRole("status");

    expect(status.className).toContain("bg-success-container");
    expect(
      screen.getByRole("heading", { name: "Action completed" }),
    ).toBeTruthy();
    expect(screen.getByText("Host-provided success message.")).toBeTruthy();
  });
});

describe("WidgetFrameDisclosure", () => {
  it("exposes disclosure semantics and toggles expanded state from callbacks", async () => {
    const user = userEvent.setup();
    const onExpandedChange = vi.fn();

    render(
      <WidgetFrameDisclosure>
        <WidgetFrameDisclosureTrigger
          controlsID="details-panel"
          expanded={false}
          onExpandedChange={onExpandedChange}
        >
          Show details
        </WidgetFrameDisclosureTrigger>
        <WidgetFrameDisclosurePanel expanded={false} id="details-panel">
          <p>Hidden details</p>
        </WidgetFrameDisclosurePanel>
      </WidgetFrameDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: "Show details" });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(trigger.getAttribute("aria-controls")).toBe("details-panel");
    expect(trigger.querySelector("svg")).toBeTruthy();
    expect(
      document.getElementById("details-panel")?.hasAttribute("hidden"),
    ).toBe(true);

    await user.click(trigger);

    expect(onExpandedChange).toHaveBeenCalledWith(true);
  });
});

describe("Widget frame layout helpers", () => {
  it("builds responsive story shell styles for bounded viewports", () => {
    expect(widgetFrameStoryShellStyle("360px")).toEqual({
      style: {
        maxWidth: "360px",
        padding: "1rem",
        width: "100%",
      },
    });
  });

  it("detects horizontal overflow within the configured tolerance", () => {
    const element = document.createElement("div");
    Object.defineProperty(element, "scrollWidth", { value: 120 });
    Object.defineProperty(element, "clientWidth", { value: 100 });

    expect(widgetFrameHasNoHorizontalOverflow(element)).toBe(false);
    expect(widgetFrameHasNoHorizontalOverflow(element, 20)).toBe(true);
  });
});
