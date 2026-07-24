// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { Code, Heading, Label, Text } from "./typography";
import {
  CAPTION_TEXT_CLASS,
  DENSE_BODY_TEXT_CLASS,
  MUTED_TEXT_CLASS,
  TEXT_TRUNCATE_CLASS,
  TEXT_WRAP_CLASS,
} from "./typography-roles";

const LONG_LABEL =
  "Extremely long host-supplied label that should not force horizontal overflow";

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

  it("renders muted, caption, and dense text roles with semantic tokens", () => {
    renderPackageComponent(
      <>
        <Text variant="muted">Muted metadata</Text>
        <Text variant="caption">Caption copy</Text>
        <Text variant="dense">Dense metadata row</Text>
      </>,
    );

    expect(screen.getByText("Muted metadata").className).toContain(
      MUTED_TEXT_CLASS,
    );
    expect(screen.getByText("Caption copy").className).toContain(
      CAPTION_TEXT_CLASS,
    );
    expect(screen.getByText("Dense metadata row").className).toContain(
      DENSE_BODY_TEXT_CLASS,
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

  it("applies truncation classes for long labels and values", () => {
    renderPackageComponent(
      <div style={{ width: "120px" }}>
        <Label truncate>{LONG_LABEL}</Label>
        <Text truncate>{LONG_LABEL}</Text>
      </div>,
    );

    expect(screen.getAllByText(LONG_LABEL)).toHaveLength(2);
    for (const element of screen.getAllByText(LONG_LABEL)) {
      expect(element.className).toContain(TEXT_TRUNCATE_CLASS);
    }
  });

  it("applies wrapping classes for long body copy", () => {
    renderPackageComponent(<Text wrap>{LONG_LABEL}</Text>);

    expect(screen.getByText(LONG_LABEL).className).toContain(TEXT_WRAP_CLASS);
  });

  it("truncates long section headings inside constrained containers", () => {
    renderPackageComponent(
      <div style={{ width: "120px" }}>
        <Heading level="section" truncate>
          {LONG_LABEL}
        </Heading>
      </div>,
    );

    expect(
      screen.getByRole("heading", { name: LONG_LABEL }).className,
    ).toContain(TEXT_TRUNCATE_CLASS);
  });
});
