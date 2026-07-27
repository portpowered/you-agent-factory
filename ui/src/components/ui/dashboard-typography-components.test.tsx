import { render, screen } from "@testing-library/react";

import { Code, Heading, Label, Text } from "@you-agent-factory/components/primitives";

describe("dashboard typography components", () => {
  it("renders page and section heading roles", () => {
    render(
      <>
        <Heading level="page">Factory dashboard</Heading>
        <Heading as="h4">Runtime details</Heading>
      </>,
    );

    expect(
      screen.getByRole("heading", { level: 1, name: "Factory dashboard" })
        .className,
    ).toContain("af-page-heading");
    expect(
      screen.getByRole("heading", { level: 4, name: "Runtime details" })
        .className,
    ).toContain("af-section-heading");
  });

  it("renders body and supporting text roles on configurable elements", () => {
    render(
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
    render(
      <>
        <Label as="dt">Dispatch ID</Label>
        <Code size="supporting">dispatch-1</Code>
      </>,
    );

    expect(screen.getByText("Dispatch ID").tagName).toBe("DT");
    expect(screen.getByText("Dispatch ID").className).toContain(
      "af-supporting-label",
    );
    expect(screen.getByText("dispatch-1").className).toContain(
      "af-supporting-code",
    );
  });
});
