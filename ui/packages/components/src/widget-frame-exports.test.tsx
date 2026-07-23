// @vitest-environment happy-dom

import {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetErrorState,
  WidgetFrame,
  WidgetFrameDisclosureTrigger,
  WidgetLoadingState,
  WidgetSubtitle,
  WidgetSuccessState,
  widgetFrameDetailCardClass,
  widgetFrameHasNoHorizontalOverflow,
  widgetFrameStoryShellStyle,
} from "@you-agent-factory/components";
import * as recipes from "@you-agent-factory/components/recipes";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "./testing/render";

describe("@you-agent-factory/components widget frame exports", () => {
  it("imports widget frame contracts from the package root", () => {
    expect(WidgetFrame).toBeTypeOf("function");
    expect(WidgetSubtitle).toBeTypeOf("function");
    expect(WidgetDetailCopy).toBeTypeOf("function");
    expect(WidgetEmptyState).toBeTypeOf("function");
    expect(WidgetEmptyStateTitle).toBeTypeOf("function");
    expect(WidgetEmptyStateText).toBeTypeOf("function");
    expect(WidgetLoadingState).toBeTypeOf("function");
    expect(WidgetErrorState).toBeTypeOf("function");
    expect(WidgetSuccessState).toBeTypeOf("function");
    expect(WidgetFrameDisclosureTrigger).toBeTypeOf("object");
    expect(widgetFrameDetailCardClass).toContain("[&_dl]:grid");
    expect(widgetFrameStoryShellStyle("360px").style.maxWidth).toBe("360px");
    expect(widgetFrameHasNoHorizontalOverflow).toBeTypeOf("function");
  });

  it("imports the same contracts from the recipes category entrypoint", () => {
    expect(recipes.WidgetFrame).toBe(WidgetFrame);
    expect(recipes.WidgetSubtitle).toBe(WidgetSubtitle);
    expect(recipes.WidgetDetailCopy).toBe(WidgetDetailCopy);
    expect(recipes.WidgetEmptyState).toBe(WidgetEmptyState);
    expect(recipes.WidgetLoadingState).toBe(WidgetLoadingState);
    expect(recipes.WidgetFrameDisclosureTrigger).toBe(
      WidgetFrameDisclosureTrigger,
    );
    expect(recipes.widgetFrameDetailCardClass).toBe(widgetFrameDetailCardClass);
  });

  it("renders exported widget frame contracts without dashboard providers", () => {
    renderPackageComponent(
      <WidgetFrame title="Package widget">
        <WidgetSubtitle>42 items</WidgetSubtitle>
        <WidgetDetailCopy>Host-provided detail copy.</WidgetDetailCopy>
        <WidgetEmptyState>
          <WidgetEmptyStateTitle>No data</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>
            Provide content from the host.
          </WidgetEmptyStateText>
        </WidgetEmptyState>
      </WidgetFrame>,
    );

    expect(
      screen.getByRole("article", { name: "Package widget" }),
    ).toBeInTheDocument();
    expect(screen.getByText("42 items")).toBeInTheDocument();
    expect(screen.getByText("Host-provided detail copy.")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "No data" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Provide content from the host."),
    ).toBeInTheDocument();
  });
});
