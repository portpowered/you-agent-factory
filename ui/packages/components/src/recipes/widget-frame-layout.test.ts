// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import {
  WIDGET_FRAME_MIN_WIDTH_CLASS,
  WIDGET_FRAME_OVERFLOW_TOLERANCE_PX,
  WIDGET_FRAME_RESPONSIVE_SHELL_CLASS,
  WIDGET_FRAME_STORY_SHELL_DATA_ATTR,
  widgetFrameDetailCardClass,
  widgetFrameHasNoHorizontalOverflow,
  widgetFrameStoryShellStyle,
} from "./widget-frame-layout";

describe("widget frame layout helpers", () => {
  it("keeps the responsive shell and detail-card recipe classes stable", () => {
    expect(WIDGET_FRAME_MIN_WIDTH_CLASS).toBe("min-w-0");
    expect(WIDGET_FRAME_RESPONSIVE_SHELL_CLASS).toContain("min-w-0");
    expect(WIDGET_FRAME_RESPONSIVE_SHELL_CLASS).toContain("w-full");
    expect(widgetFrameDetailCardClass).toContain("[&_dl]:grid");
  });

  it("builds bounded story shells for compact, medium, and wide examples", () => {
    expect(widgetFrameStoryShellStyle("360px")).toEqual({
      style: {
        maxWidth: "360px",
        padding: "1rem",
        width: "100%",
      },
    });
    expect(widgetFrameStoryShellStyle("768px").style.maxWidth).toBe("768px");
    expect(widgetFrameStoryShellStyle("1280px").style.maxWidth).toBe("1280px");
  });

  it("detects horizontal overflow within the configured tolerance", () => {
    const shell = document.createElement("div");
    shell.setAttribute(WIDGET_FRAME_STORY_SHELL_DATA_ATTR, "true");
    Object.defineProperty(shell, "clientWidth", {
      configurable: true,
      value: 360,
    });
    Object.defineProperty(shell, "scrollWidth", {
      configurable: true,
      value: 360,
    });

    expect(
      widgetFrameHasNoHorizontalOverflow(
        shell,
        WIDGET_FRAME_OVERFLOW_TOLERANCE_PX,
      ),
    ).toBe(true);

    Object.defineProperty(shell, "scrollWidth", {
      configurable: true,
      value: 366,
    });

    expect(
      widgetFrameHasNoHorizontalOverflow(
        shell,
        WIDGET_FRAME_OVERFLOW_TOLERANCE_PX,
      ),
    ).toBe(false);
    expect(widgetFrameHasNoHorizontalOverflow(shell, 6)).toBe(true);
  });
});
