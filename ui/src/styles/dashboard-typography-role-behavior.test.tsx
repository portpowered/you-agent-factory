import { render, screen } from "@testing-library/react";
import { Code, Heading, Label, Text } from "../components/ui";
import {
  DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES,
  DASHBOARD_TYPOGRAPHY_CONTRACT,
} from "../components/ui/dashboard-typography";

describe("dashboard typography role behavior", () => {
  it("renders contract typography roles on representative dashboard primitives", () => {
    render(
      <>
        <Heading level="page">Page title</Heading>
        <Heading as="h4">Section title</Heading>
        <Text>Body copy</Text>
        <Text as="span" variant="supporting">
          Supporting copy
        </Text>
      </>,
    );

    for (const entry of DASHBOARD_TYPOGRAPHY_CONTRACT) {
      const node = screen.getByText(
        entry.role === "pageHeading"
          ? "Page title"
          : entry.role === "sectionHeading"
            ? "Section title"
            : entry.role === "bodyText"
              ? "Body copy"
              : "Supporting copy",
      );
      expect(node.className).toContain(entry.className);
    }
  });

  it("renders extended label and code typography roles", () => {
    render(
      <>
        <Label as="dt">Field label</Label>
        <Code size="body">payload.json</Code>
        <Code size="supporting">trace-id</Code>
      </>,
    );

    expect(screen.getByText("Field label").className).toContain(
      DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES[0].className,
    );
    expect(screen.getByText("payload.json").className).toContain(
      DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES[1].className,
    );
    expect(screen.getByText("trace-id").className).toContain(
      DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES[2].className,
    );
  });
});
