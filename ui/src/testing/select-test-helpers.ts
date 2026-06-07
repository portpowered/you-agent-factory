import {
  type BoundFunctions,
  fireEvent,
  type queries,
  screen,
  within,
} from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";

type ScreenScope = BoundFunctions<typeof queries>;

export async function selectComboboxOption(
  user: UserEvent,
  trigger: HTMLElement,
  optionName: string,
) {
  await user.click(trigger);
  const listbox = await screen.findByRole("listbox");
  await user.click(within(listbox).getByRole("option", { name: optionName }));
}

export async function selectLabeledComboboxOption(
  user: UserEvent,
  label: string | RegExp,
  optionName: string,
  scope: ScreenScope = screen,
) {
  const trigger = scope.getByLabelText(label);
  await selectComboboxOption(user, trigger, optionName);
}

export async function clickLabeledComboboxOption(
  label: string | RegExp,
  optionName: string,
  scope: ScreenScope = screen,
) {
  const trigger = scope.getByLabelText(label);
  fireEvent.click(trigger);
  const listbox = await screen.findByRole("listbox");
  fireEvent.click(within(listbox).getByRole("option", { name: optionName }));
}
