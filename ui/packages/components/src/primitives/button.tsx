import { Slot } from "@radix-ui/react-slot";
import {
  type ButtonHTMLAttributes,
  Children,
  cloneElement,
  forwardRef,
  isValidElement,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
  type Ref,
} from "react";

import { cn } from "../utilities/cn";

const BUTTON_LOADING_CONTENT_CLASS =
  "inline-flex items-center justify-center gap-2";
const BUTTON_LOADING_HIDDEN_CONTENT_CLASS = "opacity-0";
const BUTTON_LOADING_OVERLAY_CLASS =
  "pointer-events-none absolute inset-0 inline-flex items-center justify-center";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean;
  loading?: boolean;
  tone?:
    | "default"
    | "destructive"
    | "outline"
    | "secondary"
    | "ghost"
    | "warning";
  size?: "default" | "icon" | "iconPill" | "lg" | "pill" | "sm";
}

const BUTTON_BASE_CLASS =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-xl border font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-focus-ring focus-visible:ring-offset-0 disabled:pointer-events-none disabled:border-outline disabled:bg-surface-container-low disabled:text-on-surface-disabled";
const BUTTON_TONE_CLASS: Record<NonNullable<ButtonProps["tone"]>, string> = {
  default:
    "border-primary bg-primary text-on-primary hover:border-on-primary-container hover:bg-on-primary-container",
  destructive:
    "border-error bg-error text-on-error hover:border-af-danger-hover hover:bg-af-danger-hover",
  ghost:
    "border-transparent bg-transparent text-on-surface-variant hover:bg-af-overlay hover:text-on-surface",
  outline:
    "border-outline bg-surface-container-high text-on-surface hover:border-outline-variant hover:bg-af-overlay",
  secondary:
    "border-outline-variant bg-surface-container-low text-primary hover:border-primary hover:bg-af-overlay",
  warning:
    "border-af-warning-border bg-warning-container text-on-warning-container hover:border-af-warning-border hover:bg-warning-container hover:text-on-warning-container",
};
const BUTTON_SIZE_CLASS: Record<NonNullable<ButtonProps["size"]>, string> = {
  default: "px-4 py-2.5 text-sm",
  icon: "h-11 w-11 px-0 py-0",
  iconPill: "h-10 min-h-10 w-10 rounded-full px-0 py-0",
  lg: "px-5 py-3 text-base",
  pill: "min-h-9 rounded-full px-3 py-2 text-xs",
  sm: "min-h-9 rounded-lg px-3 py-2 text-xs",
};

export const buttonVariants = ({
  className,
  size = "default",
  tone = "default",
}: Pick<ButtonProps, "className" | "size" | "tone">) =>
  cn(
    BUTTON_BASE_CLASS,
    BUTTON_TONE_CLASS[tone],
    BUTTON_SIZE_CLASS[size],
    className,
  );

function ButtonLoadingSpinner() {
  return (
    <svg
      aria-hidden="true"
      className="size-4 animate-spin"
      fill="none"
      focusable="false"
      viewBox="0 0 16 16"
    >
      <circle
        className="text-on-surface-disabled"
        cx="8"
        cy="8"
        r="6"
        stroke="currentColor"
        strokeWidth="1.5"
      />
      <path
        d="M8 2a6 6 0 0 1 6 6"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}

function renderButtonContent(children: ReactNode, loading: boolean): ReactNode {
  if (!loading) {
    return children;
  }

  return (
    <>
      <span
        className={cn(
          BUTTON_LOADING_CONTENT_CLASS,
          BUTTON_LOADING_HIDDEN_CONTENT_CLASS,
        )}
      >
        {children}
      </span>
      <span aria-hidden="true" className={BUTTON_LOADING_OVERLAY_CLASS}>
        <ButtonLoadingSpinner />
      </span>
    </>
  );
}

function suppressSlottedActivation(event: KeyboardEvent | MouseEvent): void {
  event.preventDefault();
  const nativeEvent = event.nativeEvent;
  if (typeof nativeEvent.stopImmediatePropagation === "function") {
    nativeEvent.stopImmediatePropagation();
    return;
  }
  event.stopPropagation();
}

function shouldSuppressSlottedKeyboardActivation(
  event: KeyboardEvent,
): boolean {
  return event.key === "Enter" || event.key === " ";
}

type SlottedChildProps = {
  "aria-busy"?: boolean | "true" | "false";
  "aria-disabled"?: boolean | "true" | "false";
  className?: string;
  href?: string;
  onClick?: (event: MouseEvent<HTMLElement>) => void;
  onKeyDown?: (event: KeyboardEvent<HTMLElement>) => void;
  ref?: Ref<HTMLElement>;
};

function mergeRefs<T>(...refs: Array<Ref<T> | undefined>): Ref<T> {
  return (value) => {
    for (const ref of refs) {
      if (typeof ref === "function") {
        ref(value);
      } else if (ref && typeof ref === "object") {
        ref.current = value;
      }
    }
  };
}

function renderBlockedAsChildButton({
  children,
  className,
  loading,
  props,
  ref,
  size,
  tone,
}: {
  children: ReactNode;
  className?: string;
  loading: boolean;
  props: Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children">;
  ref: Ref<HTMLButtonElement>;
  size: NonNullable<ButtonProps["size"]>;
  tone: NonNullable<ButtonProps["tone"]>;
}) {
  const child = Children.only(children);
  if (!isValidElement<SlottedChildProps>(child)) {
    throw new Error(
      "Button with asChild requires a single React element child.",
    );
  }

  return cloneElement(child, {
    ...props,
    "aria-busy": loading || undefined,
    "aria-disabled": true,
    className: buttonVariants({
      className: cn("pointer-events-none", className, child.props.className),
      size,
      tone,
    }),
    href: undefined,
    onClick: suppressSlottedActivation,
    onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
      if (shouldSuppressSlottedKeyboardActivation(event)) {
        suppressSlottedActivation(event);
      }
    },
    ref: mergeRefs(ref, child.props.ref),
  });
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(
    {
      asChild = false,
      children,
      className,
      disabled,
      loading = false,
      onClick,
      onKeyDown,
      size = "default",
      tone = "default",
      type = "button",
      ...props
    },
    ref,
  ) {
    const isInteractionBlocked = disabled || loading;
    const showLoadingOverlay = loading && !asChild;

    if (asChild && isInteractionBlocked) {
      return renderBlockedAsChildButton({
        children,
        className,
        loading,
        props,
        ref,
        size,
        tone,
      });
    }

    const Component = asChild ? Slot : "button";

    return (
      <Component
        aria-busy={loading || undefined}
        className={buttonVariants({
          className: cn(showLoadingOverlay && "relative", className),
          size,
          tone,
        })}
        disabled={asChild ? undefined : isInteractionBlocked}
        onClick={onClick}
        onKeyDown={onKeyDown}
        ref={ref}
        {...(!asChild ? { type } : undefined)}
        {...props}
      >
        {renderButtonContent(children, showLoadingOverlay)}
      </Component>
    );
  },
);
