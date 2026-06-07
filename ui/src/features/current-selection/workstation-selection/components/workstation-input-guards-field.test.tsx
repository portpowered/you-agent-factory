import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { selectComboboxOption } from "../../../../testing/select-test-helpers";

import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { EditableConfigurationWorkstationInputGuardsField } from "./workstation-input-guards-field";

const messages = getWorkstationDetailMessages("en");

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("EditableConfigurationWorkstationInputGuardsField", () => {
  it("renders each input slot with work type, state, and guard summary", () => {
    render(
      <EditableConfigurationWorkstationInputGuardsField
        inputs={[
          { guards: [], state: "queued", workType: "story" },
          {
            guards: [{ matchInput: "story", type: "SAME_NAME" }],
            state: "complete",
            workType: "task",
          },
        ]}
        messages={messages}
        onInputsChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Input guards" })).toBeTruthy();
    const slotArticles = screen.getAllByRole("article");
    expect(
      within(slotArticles[0]).getByRole("heading", { level: 6 }).textContent,
    ).toBe("story · queued");
    expect(
      within(slotArticles[1]).getByRole("heading", { level: 6 }).textContent,
    ).toBe("task · complete");
    expect(within(slotArticles[1]).getByText("Same name · story")).toBeTruthy();
  });

  it("updates the draft when guard type changes and clears a guard", async () => {
    const user = userEvent.setup();
    const onInputsChange = vi.fn();

    const { rerender } = render(
      <EditableConfigurationWorkstationInputGuardsField
        inputs={[
          { guards: [], state: "queued", workType: "story" },
          { guards: [], state: "complete", workType: "task" },
        ]}
        messages={messages}
        onInputsChange={onInputsChange}
      />,
    );

    const slotArticles = screen.getAllByRole("article");
    await selectComboboxOption(
      user,
      within(slotArticles[1]).getByLabelText("Input guard"),
      "Same name",
    );

    expect(onInputsChange).toHaveBeenCalledWith([
      { guards: [], state: "queued", workType: "story" },
      {
        guards: [{ matchInput: "story", type: "SAME_NAME" }],
        state: "complete",
        workType: "task",
      },
    ]);

    onInputsChange.mockClear();
    rerender(
      <EditableConfigurationWorkstationInputGuardsField
        inputs={[
          { guards: [], state: "queued", workType: "story" },
          {
            guards: [{ matchInput: "story", type: "SAME_NAME" }],
            state: "complete",
            workType: "task",
          },
        ]}
        messages={messages}
        onInputsChange={onInputsChange}
      />,
    );

    const updatedSlotArticles = screen.getAllByRole("article");
    await selectComboboxOption(
      user,
      within(updatedSlotArticles[1]).getByLabelText("Input guard"),
      messages.workstationInputGuardNoneOption,
    );

    expect(onInputsChange).toHaveBeenCalledWith([
      { guards: [], state: "queued", workType: "story" },
      { guards: [], state: "complete", workType: "task" },
    ]);
  });

  it("shows parent and spawned-by fields for parent-aware guards", async () => {
    const user = userEvent.setup();
    const onInputsChange = vi.fn();

    render(
      <EditableConfigurationWorkstationInputGuardsField
        inputs={[
          { guards: [], state: "queued", workType: "story" },
          { guards: [], state: "complete", workType: "task" },
        ]}
        messages={messages}
        onInputsChange={onInputsChange}
      />,
    );

    const slotArticles = screen.getAllByRole("article");
    await selectComboboxOption(
      user,
      within(slotArticles[1]).getByLabelText("Input guard"),
      "All children complete",
    );

    expect(onInputsChange).toHaveBeenCalledWith([
      { guards: [], state: "queued", workType: "story" },
      {
        guards: [{ parentInput: "story", type: "ALL_CHILDREN_COMPLETE" }],
        state: "complete",
        workType: "task",
      },
    ]);
  });
});
