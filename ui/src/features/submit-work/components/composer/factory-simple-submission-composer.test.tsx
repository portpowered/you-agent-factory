import "@testing-library/jest-dom/vitest";
import {
  createEvent,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

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
    onSubmit: vi.fn().mockResolvedValue(undefined),
    workTypes: [
      { handlingBehavior: ["DEFAULT"], isSubmitEligible: true, name: "task" },
    ],
    ...overrides,
  };
  render(<FactorySimpleSubmissionComposer {...props} />);
  return props;
}

function renderUnavailableComposers() {
  render(
    <>
      <FactorySimpleSubmissionComposer
        draft="First task"
        factoryState="active"
        isCurrent={false}
        onDraftChange={vi.fn()}
        onSubmit={vi.fn().mockResolvedValue(undefined)}
        workTypes={[
          {
            handlingBehavior: ["DEFAULT"],
            isSubmitEligible: true,
            name: "first",
          },
        ]}
      />
      <FactorySimpleSubmissionComposer
        draft="Second task"
        factoryState="active"
        isCurrent={false}
        onDraftChange={vi.fn()}
        onSubmit={vi.fn().mockResolvedValue(undefined)}
        workTypes={[
          {
            handlingBehavior: ["DEFAULT"],
            isSubmitEligible: true,
            name: "second",
          },
        ]}
      />
    </>,
  );
}

describe("FactorySimpleSubmissionComposer", () => {
  it("submits one text content item for the resolved work type without a name or relationships", async () => {
    const props = renderComposer();
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    await waitFor(() => {
      expect(props.onSubmit).toHaveBeenCalledWith({
        content: [{ text: "A new task", type: "text" }],
        workTypeName: "task",
      });
    });
    await waitFor(() => expect(props.onDraftChange).toHaveBeenCalledOnce());
    expect(props.onDraftChange).toHaveBeenCalledWith("");
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

  it("keeps labels and unavailable status text isolated across composer instances", () => {
    renderUnavailableComposers();

    const labels = Array.from(document.querySelectorAll("label"));
    const textareas = screen.getAllByRole("textbox", { name: "Submit text" });
    const statuses = screen.getAllByRole("status");

    expect(new Set(textareas.map((textarea) => textarea.id)).size).toBe(2);
    expect(new Set(statuses.map((status) => status.id)).size).toBe(2);
    textareas.forEach((textarea, index) => {
      expect(labels[index]).toHaveAttribute("for", textarea.id);
      expect(textarea).toHaveAttribute("aria-describedby", statuses[index].id);
    });
  });

  it("submits on Enter but preserves a newline for Shift+Enter", async () => {
    const props = renderComposer();
    const textarea = screen.getByRole("textbox", { name: "Submit text" });

    const shiftEnter = createEvent.keyDown(textarea, {
      key: "Enter",
      shiftKey: true,
    });
    fireEvent(textarea, shiftEnter);
    expect(shiftEnter.defaultPrevented).toBe(false);
    expect(props.onSubmit).not.toHaveBeenCalled();

    fireEvent.keyDown(textarea, { key: "Enter" });
    await waitFor(() => expect(props.onSubmit).toHaveBeenCalledOnce());
  });

  it("clears the draft once after success and preserves it with recoverable feedback after failure", async () => {
    const failure = new Error("The host could not submit the work.");
    const props = renderComposer({
      onSubmit: vi.fn().mockRejectedValue(failure),
    });

    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(failure.message);
    });
    expect(props.onDraftChange).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox", { name: "Submit text" })).toHaveValue(
      "A new task",
    );
  });

  it("uses a bounded auto-growing multiline field and stacks safely on narrow viewports", () => {
    renderComposer();
    const form = screen.getByRole("form", { name: "Simple work submission" });
    const textarea = screen.getByRole("textbox", { name: "Submit text" });

    expect(form.className).toContain("sm:grid-cols-[minmax(0,1fr)_auto]");
    expect(textarea.className).toContain("min-h-24");
    expect(textarea.className).toContain("max-h-48");
    expect(textarea.className).toContain("resize-none");
  });
});
