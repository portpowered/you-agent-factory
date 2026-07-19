import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";

import {
  FactorySubmissionComposer,
  type FactorySubmissionComposerProps,
} from ".";

function createProps(
  overrides: Partial<FactorySubmissionComposerProps> = {},
): FactorySubmissionComposerProps {
  return {
    draft: {
      items: [
        {
          id: "text-1",
          text: "Host supplied text",
          type: "text",
        },
      ],
      requestName: "Host supplied request",
      workTypeName: "story",
    },
    onAddItem: vi.fn(),
    onItemTextChange: vi.fn(),
    onRemoveItem: vi.fn(),
    onRequestNameChange: vi.fn(),
    onStageFileItems: vi.fn(),
    onSubmit: vi.fn(),
    onWorkTypeNameChange: vi.fn(),
    status: { kind: "guidance", message: "Ready" },
    submitWorkTypeNames: ["story"],
    ...overrides,
  };
}

describe("FactorySubmissionComposer", () => {
  it("renders rich host-owned draft controls and delegates their callbacks", () => {
    const props = createProps();

    render(<FactorySubmissionComposer {...props} />);

    const card = screen.getByRole("article", { name: "Submit work" });
    const requestName = within(card).getByRole("textbox", {
      name: /Request name/,
    });
    fireEvent.change(requestName, { target: { value: "Updated request" } });
    const form = within(card)
      .getByRole("button", { name: "Submit work" })
      .closest("form");
    if (!(form instanceof HTMLFormElement)) {
      throw new Error("Expected the submit action to belong to a form.");
    }
    fireEvent.submit(form);

    expect(props.onRequestNameChange).toHaveBeenLastCalledWith(
      "Updated request",
    );
    expect(props.onSubmit).toHaveBeenCalledOnce();
    expect(
      within(card).getByRole("button", { name: "Add input" }),
    ).toBeEnabled();
  });

  it("renders host-supplied validation and submission failure state", () => {
    render(
      <FactorySubmissionComposer
        {...createProps({
          status: { kind: "error", message: "The host rejected this request." },
          validationErrors: { requestName: "A request name is required." },
        })}
      />,
    );

    expect(screen.getByText("The host rejected this request.")).toBeVisible();
    expect(screen.getByText("A request name is required.")).toBeVisible();
  });
});
