// @vitest-environment happy-dom

import {
  ActionRow,
  DescriptionList,
  Heading,
  Label,
  SurfacePanel,
  Text,
} from "@you-agent-factory/components";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "./testing/render";

describe("typography and display primitives package import surface", () => {
  it("renders representative typography, surface, action-row, and description-list examples", () => {
    renderPackageComponent(
      <SurfacePanel className="grid gap-3" radius="lg">
        <Heading level="section">Details</Heading>
        <DescriptionList>
          <div>
            <Label as="dt">Name</Label>
            <Text as="dd">Example value</Text>
          </div>
        </DescriptionList>
        <ActionRow
          actions={<button type="button">Primary action</button>}
          statuses={<span>Status copy</span>}
        />
      </SurfacePanel>,
    );

    expect(
      screen.getByRole("heading", { name: "Details" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Example value")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Primary action" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Status copy")).toBeInTheDocument();
  });
});
