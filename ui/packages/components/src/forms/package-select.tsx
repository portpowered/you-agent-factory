import * as SelectPrimitive from "@radix-ui/react-select";
import {
  type ComponentProps,
  type ComponentRef,
  forwardRef,
  type ReactNode,
} from "react";

import { cn } from "../utilities/cn";
import { inputVariants } from "./package-input";
import { SelectCheckIcon, SelectChevronDownIcon } from "./select-icons";

export const Select = SelectPrimitive.Root;
export const SelectGroup = SelectPrimitive.Group;
export const SelectValue = SelectPrimitive.Value;

export type SelectTriggerProps = ComponentProps<typeof SelectPrimitive.Trigger>;

export const SelectTrigger = forwardRef<
  ComponentRef<typeof SelectPrimitive.Trigger>,
  SelectTriggerProps
>(function SelectTrigger({ className, children, ...props }, ref) {
  return (
    <SelectPrimitive.Trigger
      className={cn(
        inputVariants({
          className:
            "flex items-center justify-between gap-2 text-left data-[placeholder]:text-on-surface-disabled [&>span]:line-clamp-1",
        }),
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon asChild>
        <SelectChevronDownIcon className="h-4 w-4 shrink-0 text-af-text-subtle" />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  );
});

export type SelectContentProps = ComponentProps<typeof SelectPrimitive.Content>;

// tailwind-exception: intrinsic-sizing
const SELECT_CONTENT_POPPER_CLASS = "min-w-[var(--radix-select-trigger-width)]";
// tailwind-exception: intrinsic-sizing
const SELECT_VIEWPORT_POPPER_CLASS =
  "h-[var(--radix-select-trigger-height)] w-full min-w-[var(--radix-select-trigger-width)]";

export function SelectContent({
  children,
  className,
  onCloseAutoFocus,
  position = "popper",
  ...props
}: SelectContentProps) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        className={cn(
          "relative z-50 max-h-72 overflow-hidden rounded-xl border border-outline bg-surface-container-high text-on-surface shadow-af-panel data-[state=closed]:animate-out data-[state=open]:animate-in",
          position === "popper" && SELECT_CONTENT_POPPER_CLASS,
          position === "popper" &&
            "data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1",
          className,
        )}
        onCloseAutoFocus={onCloseAutoFocus}
        position={position}
        {...props}
      >
        <SelectPrimitive.Viewport
          className={cn(
            "p-1",
            position === "popper" && SELECT_VIEWPORT_POPPER_CLASS,
          )}
        >
          {children}
        </SelectPrimitive.Viewport>
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  );
}

export type SelectLabelProps = ComponentProps<typeof SelectPrimitive.Label>;

export function SelectLabel({ className, ...props }: SelectLabelProps) {
  return (
    <SelectPrimitive.Label
      className={cn(
        "px-2 py-1.5 text-xs font-semibold uppercase tracking-[0.08em] text-on-surface-subtle",
        className,
      )}
      {...props}
    />
  );
}

export const SELECT_EMPTY_STATE_VALUE = "__select-empty-state__";

export type SelectItemProps = ComponentProps<typeof SelectPrimitive.Item>;

export const SelectItem = forwardRef<
  ComponentRef<typeof SelectPrimitive.Item>,
  SelectItemProps
>(function SelectItem({ children, className, ...props }, ref) {
  return (
    <SelectPrimitive.Item
      className={cn(
        "relative flex w-full min-w-0 cursor-default select-none items-center rounded-lg py-2 pl-8 pr-2 text-sm outline-none focus:bg-surface-container-highest data-[disabled]:pointer-events-none data-[disabled]:text-on-surface-disabled data-[highlighted]:bg-surface-container-highest",
        className,
      )}
      ref={ref}
      {...props}
    >
      <span className="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
        <SelectPrimitive.ItemIndicator>
          <SelectCheckIcon className="h-4 w-4" />
        </SelectPrimitive.ItemIndicator>
      </span>
      <SelectPrimitive.ItemText className="line-clamp-2 break-words">
        {children}
      </SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  );
});

export type SelectEmptyProps = {
  children: ReactNode;
  className?: string;
};

export function SelectEmpty({ children, className }: SelectEmptyProps) {
  return (
    <SelectPrimitive.Item
      className={cn(
        "relative flex w-full min-w-0 cursor-default select-none items-center rounded-lg px-2 py-2 text-sm text-on-surface-disabled data-[disabled]:pointer-events-none data-[disabled]:text-on-surface-disabled",
        className,
      )}
      disabled
      value={SELECT_EMPTY_STATE_VALUE}
    >
      <SelectPrimitive.ItemText className="line-clamp-2 break-words">
        {children}
      </SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  );
}

export type SelectSeparatorProps = ComponentProps<
  typeof SelectPrimitive.Separator
>;

export function SelectSeparator({ className, ...props }: SelectSeparatorProps) {
  return (
    <SelectPrimitive.Separator
      className={cn("-mx-1 my-1 h-px bg-outline", className)}
      {...props}
    />
  );
}

export interface SelectFieldProps {
  children: ReactNode;
  className?: string;
  description?: ReactNode;
  descriptionId?: string;
  error?: ReactNode;
  errorId?: string;
  inputId: string;
  label: ReactNode;
}

export function SelectField({
  children,
  className,
  description,
  descriptionId,
  error,
  errorId,
  inputId,
  label,
}: SelectFieldProps) {
  return (
    <div className={cn("grid gap-2", className)}>
      <label
        className="text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle"
        htmlFor={inputId}
      >
        {label}
      </label>
      {children}
      {description ? (
        <p className="m-0 text-sm text-af-text-muted" id={descriptionId}>
          {description}
        </p>
      ) : null}
      {error ? (
        <p
          className="m-0 text-sm font-medium text-on-error-container"
          id={errorId}
          role="alert"
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}
