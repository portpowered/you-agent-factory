// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { Code, Heading, Label, Text } from "./typography";

describe("typography primitives", () => {
  it("renders page and section heading roles", () => {
    renderPackageComponent(
      <>
        <Heading level="page">Application title</Heading>
        <Heading as="h4">Section title</Heading>
      </>,
    );

    expect(
      screen.getByRole("heading", { level: 1, name: "Application title" })
        .className,
    ).toContain("af-page-heading");
    expect(
      screen.getByRole("heading", { level: 4, name: "Section title" })
        .className,
    ).toContain("af-section-heading");
  });

  it("renders body and supporting text roles on configurable elements", () => {
    renderPackageComponent(
      <>
        <Text>Body copy</Text>
        <Text as="span" variant="supporting">
          Supporting copy
        </Text>
      </>,
    );

    expect(screen.getByText("Body copy").tagName).toBe("P");
    expect(screen.getByText("Body copy").className).toContain("af-body-text");
    expect(screen.getByText("Supporting copy").tagName).toBe("SPAN");
    expect(screen.getByText("Supporting copy").className).toContain(
      "af-supporting-text",
    );
  });

  it("renders label and code typography roles", () => {
    renderPackageComponent(
      <>
        <Label as="dt">Field label</Label>
        <Code size="supporting">value-1</Code>
      </>,
    );

    expect(screen.getByText("Field label").tagName).toBe("DT");
    expect(screen.getByText("Field label").className).toContain(
      "af-supporting-label",
    );
    expect(screen.getByText("value-1").className).toContain(
      "af-supporting-code",
    );
  });
});
