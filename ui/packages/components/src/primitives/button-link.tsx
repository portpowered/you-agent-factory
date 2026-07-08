import { type AnchorHTMLAttributes, forwardRef } from "react";

import { Button, type ButtonProps } from "./button";

export interface ButtonLinkProps
  extends AnchorHTMLAttributes<HTMLAnchorElement>,
    Pick<ButtonProps, "size" | "tone"> {}

export const ButtonLink = forwardRef<HTMLAnchorElement, ButtonLinkProps>(
  function ButtonLink(
    { children, className, size = "default", tone = "default", ...props },
    ref,
  ) {
    return (
      <Button asChild className={className} size={size} tone={tone}>
        <a ref={ref} {...props}>
          {children}
        </a>
      </Button>
    );
  },
);
