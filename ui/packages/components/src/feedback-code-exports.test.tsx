// @vitest-environment happy-dom

import {
  AlertPanel,
  AlertPanelText,
  CodePanel,
  Skeleton,
} from "@you-agent-factory/components";
import * as dataDisplay from "@you-agent-factory/components/data-display";
import * as feedback from "@you-agent-factory/components/feedback";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "./testing/render";

describe("@you-agent-factory/components feedback and code exports", () => {
  it("imports AlertPanel, Skeleton, and CodePanel from the package root", () => {
    expect(AlertPanel).toBeTypeOf("object");
    expect(AlertPanelText).toBeTypeOf("object");
    expect(Skeleton).toBeTypeOf("function");
    expect(CodePanel).toBeTypeOf("object");
  });

  it("imports the same primitives from category entrypoints", () => {
    expect(feedback.AlertPanel).toBe(AlertPanel);
    expect(feedback.Skeleton).toBe(Skeleton);
    expect(dataDisplay.CodePanel).toBe(CodePanel);
  });

  it("renders exported feedback and code primitives without dashboard providers", () => {
    renderPackageComponent(
      <>
        <AlertPanel tone="info">Package alert</AlertPanel>
        <Skeleton data-testid="package-skeleton" />
        <CodePanel>package code</CodePanel>
      </>,
    );

    expect(screen.getByText("Package alert")).toBeInTheDocument();
    expect(screen.getByTestId("package-skeleton")).toBeInTheDocument();
    expect(screen.getByText("package code")).toBeInTheDocument();
  });
});
