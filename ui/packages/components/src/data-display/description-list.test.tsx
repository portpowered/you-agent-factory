// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { Label, Text } from "../primitives/typography";
import { renderPackageComponent, screen } from "../testing/render";
import { DescriptionList } from "./description-list";

const LONG_LABEL =
  "Extremely long host-supplied label that should remain readable without forcing horizontal page overflow";
const LONG_VALUE =
  "Host-supplied value with a very long identifier or message that should wrap within the description-list layout instead of clipping or overflowing the page";

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

  it("renders dense rows with host-supplied compact copy", () => {
    renderPackageComponent(
      <DescriptionList className="gap-1">
        <div>
          <Label as="dt">Revision</Label>
          <Text as="dd" variant="dense">
            Dense metadata row value
          </Text>
        </div>
      </DescriptionList>,
    );

    const value = screen.getByText("Dense metadata row value");
    expect(value.className).toContain("af-dense-body-text");
  });

  it("renders long labels and wrapped values supplied by the host app", () => {
    renderPackageComponent(
      <DescriptionList className="[&_div]:grid-cols-[8.5rem_minmax(0,1fr)]">
        <div>
          <Label as="dt" truncate>
            {LONG_LABEL}
          </Label>
          <Text as="dd" wrap>
            {LONG_VALUE}
          </Text>
        </div>
      </DescriptionList>,
    );

    const label = screen.getByText(LONG_LABEL);
    const value = screen.getByText(LONG_VALUE);

    expect(label.className).toContain("af-text-truncate");
    expect(value.className).toContain("af-text-wrap");
  });

  it("renders host-supplied empty or missing value placeholders", () => {
    renderPackageComponent(
      <DescriptionList>
        <div>
          <Label as="dt">Trace ID</Label>
          <Text as="dd">—</Text>
        </div>
        <div>
          <Label as="dt">Owner</Label>
          <Text as="dd" />
        </div>
      </DescriptionList>,
    );

    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.getByText("Owner").nextElementSibling?.tagName).toBe("DD");
  });

  it("keeps description-list rows readable in narrow containers", () => {
    const { container } = renderPackageComponent(
      <div style={{ width: "240px" }}>
        <DescriptionList className="[&_div]:grid-cols-[7rem_minmax(0,1fr)]">
          <div>
            <Label as="dt">Status</Label>
            <Text as="dd" wrap>
              {LONG_VALUE}
            </Text>
          </div>
        </DescriptionList>
      </div>,
    );

    const list = container.querySelector("dl");
    const value = screen.getByText(LONG_VALUE);

    expect(list?.className).toContain("min-w-0");
    expect(list?.className).toContain("[&_div]:min-w-0");
    expect(list?.className).toContain("[&_dd]:min-w-0");
    expect(value.className).toContain("af-text-wrap");
  });
});
