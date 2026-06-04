import { forwardRef } from "react";

import { Button, type ButtonProps } from "./button";
import { cn } from "../../lib/cn";

export interface DisclosureButtonProps extends ButtonProps {
  controlsID: string;
  expanded: boolean;
}

export const DisclosureButton = forwardRef<
  HTMLButtonElement,
  DisclosureButtonProps
>(function DisclosureButton({ controlsID, expanded, ...props }, ref) {
  return (
    <Button
      aria-controls={controlsID}
      aria-expanded={expanded}
      ref={ref}
      tone="outline"
      {...props}
    />
  );
});
