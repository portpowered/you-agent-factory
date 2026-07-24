import { render, screen } from "@testing-library/react";

import {
  CurrentSelectionDetailCode,
  CurrentSelectionDetailItem,
  CurrentSelectionDetailValue,
} from "./current-selection-detail-item";

describe("CurrentSelectionDetailItem", () => {
  it("renders labeled detail values with long-value wrapping", () => {
    render(
      <CurrentSelectionDetailItem label="Dispatch ID" value="dispatch-1" />,
    );

    expect(screen.getByText("Dispatch ID")).toBeTruthy();
    expect(screen.getByText("dispatch-1").closest("dd")?.className).toContain(
      "[overflow-wrap:anywhere]",
    );
  });

  it("renders code values with dashboard code typography", () => {
    render(
      <CurrentSelectionDetailItem code label="Request ID" value="request-1" />,
    );

    expect(screen.getByText("request-1").className).toContain("af-body-code");
  });

  it("exposes value and code subcomponents for custom detail rows", () => {
    render(
      <dl>
        <div>
          <dt>Trace</dt>
          <CurrentSelectionDetailValue>
            <CurrentSelectionDetailCode>trace-1</CurrentSelectionDetailCode>
          </CurrentSelectionDetailValue>
        </div>
      </dl>,
    );

    expect(screen.getByText("trace-1").closest("dd")).toBeTruthy();
  });
});
