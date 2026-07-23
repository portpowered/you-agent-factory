import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../styles.css";
import { expectStyledCheckboxInStory } from "../../testing/checkbox-story-helpers";
import { Checkbox } from "./checkbox";

function CheckboxStateShowcaseStory() {
  const [optionalSetting, setOptionalSetting] = useState(false);

  return (
    <div className="flex flex-col gap-6 p-4">
      <label
        className="inline-flex items-center gap-2"
        htmlFor="optional-setting"
      >
        <Checkbox
          checked={optionalSetting}
          id="optional-setting"
          onChange={(event) => setOptionalSetting(event.currentTarget.checked)}
        />
        Optional setting
      </label>
      <Checkbox
        aria-label="Disabled setting"
        disabled
        onChange={() => undefined}
      />
      <Checkbox
        aria-invalid="true"
        aria-label="Invalid setting"
        onChange={() => undefined}
      />
    </div>
  );
}

export default {
  title: "you-agent-factory/Checkbox Consistency/Shared Primitive",
  tags: ["test"],
};

export const CheckboxStateShowcase = {
  render: () => <CheckboxStateShowcaseStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const optionalCheckbox = await canvas.findByRole("checkbox", {
      name: "Optional setting",
    });
    const disabledCheckbox = canvas.getByRole("checkbox", {
      name: "Disabled setting",
    });
    const invalidCheckbox = canvas.getByRole("checkbox", {
      name: "Invalid setting",
    });

    for (const checkbox of [
      optionalCheckbox,
      disabledCheckbox,
      invalidCheckbox,
    ]) {
      expectStyledCheckboxInStory(checkbox);
    }

    expect(optionalCheckbox).not.toBeChecked();
    await userEvent.click(canvas.getByText("Optional setting"));
    expect(optionalCheckbox).toBeChecked();

    optionalCheckbox.focus();
    await userEvent.keyboard(" ");
    expect(optionalCheckbox).not.toBeChecked();

    expect(disabledCheckbox).toBeDisabled();
    expect(invalidCheckbox).toHaveAttribute("aria-invalid", "true");

    optionalCheckbox.focus();
    await expect(optionalCheckbox).toHaveFocus();
    await userEvent.tab();
    await expect(invalidCheckbox).toHaveFocus();
  },
};
