import * as DialogPrimitive from "@radix-ui/react-dialog";
import type { ComponentProps, HTMLAttributes } from "react";

import { cn } from "../utilities/cn";
import {
  OVERLAY_DIALOG_CONTENT_SHELL_CLASS,
  OVERLAY_FORM_GROUP_CLASS,
} from "./overlay-layout";

const DIALOG_CLOSE_BUTTON_CLASS =
  "absolute right-4 top-4 inline-flex min-h-9 w-9 items-center justify-center rounded-full border border-outline bg-transparent p-0 text-on-surface transition-colors hover:bg-surface-container-low disabled:pointer-events-none disabled:opacity-50";

function DialogCloseIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="18"
      viewBox="0 0 24 24"
      width="18"
    >
      <path
        d="M6 6l12 12M18 6L6 18"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

export const Dialog = DialogPrimitive.Root;
export const DialogTrigger = DialogPrimitive.Trigger;
export const DialogPortal = DialogPrimitive.Portal;
export const DialogClose = DialogPrimitive.Close;

export function DialogOverlay({
  className,
  ...props
}: ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      className={cn(
        "fixed inset-0 z-50 bg-black/70 backdrop-blur-sm data-[state=closed]:animate-out data-[state=open]:animate-in",
        className,
      )}
      {...props}
    />
  );
}

export type DialogContentProps = ComponentProps<
  typeof DialogPrimitive.Content
> & {
  closeDisabled?: boolean;
  closeLabel?: string;
};

export function DialogContent({
  children,
  className,
  closeDisabled = false,
  closeLabel = "Close",
  ...props
}: DialogContentProps) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        className={cn(
          "fixed inset-x-4 top-1/2 z-50 mx-auto max-h-dvh max-w-2xl -translate-y-1/2 overflow-y-auto rounded-2xl border border-outline bg-surface-container-high text-on-surface shadow-xl",
          OVERLAY_DIALOG_CONTENT_SHELL_CLASS,
          className,
        )}
        {...props}
      >
        {children}
        {closeDisabled ? (
          <button
            aria-label={closeLabel}
            className={DIALOG_CLOSE_BUTTON_CLASS}
            disabled
            type="button"
          >
            <DialogCloseIcon />
          </button>
        ) : (
          <DialogPrimitive.Close
            aria-label={closeLabel}
            className={DIALOG_CLOSE_BUTTON_CLASS}
          >
            <DialogCloseIcon />
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}

export function DialogHeader({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(OVERLAY_FORM_GROUP_CLASS, "text-left", className)}
      {...props}
    />
  );
}

export function DialogFooter({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("flex flex-wrap justify-end gap-layout-element", className)}
      {...props}
    />
  );
}

export function DialogTitle({
  className,
  ...props
}: ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      className={cn(
        "font-display text-2xl leading-tight tracking-[-0.03em] text-on-surface",
        className,
      )}
      {...props}
    />
  );
}

export function DialogDescription({
  className,
  ...props
}: ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      className={cn("text-sm leading-6 text-on-surface-variant", className)}
      {...props}
    />
  );
}
