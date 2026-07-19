import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import { FactorySimpleSubmissionComposer } from "./factory-simple-submission-composer";

function renderComposer(
  overrides: Partial<
    React.ComponentProps<typeof FactorySimpleSubmissionComposer>
  > = {},
) {
  const props = {
    draft: "A new task",
    factoryState: "active" as const,
    isCurrent: true,
    onDraftChange: vi.fn(),
    onSubmit: vi.fn(),
    workTypes: [
      { handlingBehavior: ["DEFAULT"], isSubmitEligible: true, name: "task" },
    ],
    ...overrides,
  };
  render(<FactorySimpleSubmissionComposer {...props} />);
  return props;
}

describe("FactorySimpleSubmissionComposer", () => {
  it("submits only the resolved unique eligible default", () => {
    const props = renderComposer();
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(props.onSubmit).toHaveBeenCalledWith("task");
  });

  it.each([
    [
      "missing default",
      { workTypes: [{ isSubmitEligible: true, name: "task" }] },
      /No eligible default/,
    ],
    [
      "ambiguous defaults",
      {
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            isSubmitEligible: true,
            name: "task",
          },
          {
            handlingBehavior: ["DEFAULT"],
            isSubmitEligible: true,
            name: "review",
          },
        ],
      },
      /Multiple default/,
    ],
    ["closed Factory", { factoryState: "closed" as const }, /closed/],
    ["history selection", { isCurrent: false }, /latest Factory state/],
  ])(
    "disables submission with an accessible reason for %s",
    (_caseName, overrides, message) => {
      const props = renderComposer(overrides);
      const button = screen.getByRole("button", { name: "Submit" });
      expect(button).toBeDisabled();
      expect(screen.getByRole("status")).toHaveTextContent(message);
      fireEvent.click(button);
      expect(props.onSubmit).not.toHaveBeenCalled();
    },
  );

  it("does not disable an available composer during a live dispatch", () => {
    renderComposer();
    expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();
  });
});
