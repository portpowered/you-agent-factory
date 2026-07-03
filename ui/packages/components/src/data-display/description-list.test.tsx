// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { Label, Text } from "../primitives/typography";
import { DescriptionList } from "./description-list";

describe("DescriptionList", () => {
  it("renders host-provided label and value pairs", () => {
    renderPackageComponent(
      <DescriptionList>
        <div>
          <Label as="dt">Status</Label>
          <Text as="dd">Active</Text>
        </div>
        <div>
          <Label as="dt">Owner</Label>
          <Text as="dd">Host application</Text>
        </div>
      </DescriptionList>,
    );

    const list = screen.getByText("Status").closest("dl");
    expect(list?.className).toContain("af-body-text");
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("Host application")).toBeInTheDocument();
  });
});
