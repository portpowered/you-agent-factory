import type { ReactNode } from "react";

import { Button, type ButtonProps } from "../ui/button";

export interface DashboardButtonProps extends Omit<ButtonProps, "tone"> {
  busy?: boolean;
  children: ReactNode;
  tone?: "primary" | "secondary";
}

export function DashboardButton({
  busy = false,
  children,
  tone = "primary",
  ...buttonProps
}: DashboardButtonProps) {
  return (
    <Button
      aria-busy={busy ? "true" : undefined}
      tone={tone === "primary" ? "default" : "secondary"}
      {...buttonProps}
    >
      {children}
    </Button>
  );
}
