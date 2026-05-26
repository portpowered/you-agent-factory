import { fireEvent, render, screen } from "@testing-library/react";

import { DisclosureButton } from "./disclosure-button";
import { GraphNodeButton } from "./graph-node-button";
import { SelectableCardButton } from "./selectable-card-button";

describe("semantic button wrappers", () => {
  it("keeps disclosure triggers explicit through shared expanded and controls semantics", () => {
    render(
      <DisclosureButton controlsID="details-panel" expanded type="button">
        Toggle details
      </DisclosureButton>,
    );

    expect(
      screen.getByRole("button", { name: "Toggle details" }).getAttribute(
        "aria-controls",
      ),
    ).toBe("details-panel");
    expect(
      screen.getByRole("button", { name: "Toggle details" }).getAttribute(
        "aria-expanded",
      ),
    ).toBe("true");
  });

  it("marks selectable card wrappers as pressed when selected", () => {
    render(
      <SelectableCardButton selected type="button">
        Selected row
      </SelectableCardButton>,
    );

    expect(
      screen.getByRole("button", { name: "Selected row" }).getAttribute(
        "aria-pressed",
      ),
    ).toBe("true");
  });

  it("defaults graph-node wrappers to non-submitting buttons inside forms", () => {
    const onSubmit = vi.fn((event: Event) => {
      event.preventDefault();
    });

    render(
      <form
        onSubmit={(event) => {
          onSubmit(event);
          event.preventDefault();
        }}
      >
        <GraphNodeButton>Graph node</GraphNodeButton>
      </form>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Graph node" }));

    expect(onSubmit).not.toHaveBeenCalled();
  });
});
