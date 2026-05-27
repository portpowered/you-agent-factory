import { forwardRef } from "react";

import { Button, type ButtonProps } from "./button";

export interface SelectableCardButtonProps extends ButtonProps {
  selected?: boolean;
}

export const SelectableCardButton = forwardRef<
  HTMLButtonElement,
  SelectableCardButtonProps
>(function SelectableCardButton({ selected = false, ...props }, ref) {
  return <Button aria-pressed={selected} ref={ref} {...props} />;
});
