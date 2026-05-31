import { fireEvent, render, screen } from "@testing-library/react";

import { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeStatesList } from "./work-type-states-list";

describe("WorkTypeStatesList", () => {
  const messages = getWorkTypeDetailMessages("en");

  it("emits the expected work-state graph node id when a state row is clicked", () => {
    const onSelectWorkStateGraphNode = vi.fn();

    render(
      <WorkTypeStatesList
        messages={messages}
        onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
        states={[
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ]}
        workTypeName="story"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: "Select queued state on factory graph",
      }),
    );

    expect(onSelectWorkStateGraphNode).toHaveBeenCalledWith(
      "work-state:story:queued",
    );
  });

  it("renders read-only rows without navigation controls when no handler is provided", () => {
    render(
      <WorkTypeStatesList
        messages={messages}
        states={[{ name: "queued", type: "INITIAL" }]}
        workTypeName="story"
      />,
    );

    expect(
      screen.queryByRole("button", {
        name: "Select queued state on factory graph",
      }),
    ).toBeNull();
    expect(screen.getByText("queued")).toBeTruthy();
  });
});
