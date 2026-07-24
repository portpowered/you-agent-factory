import {
  type ElementType,
  type FieldsetHTMLAttributes,
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../utilities/cn";

export type FormFieldMessageIds = {
  descriptionId?: string;
  helperId?: string;
  warningId?: string;
  errorId?: string;
  successId?: string;
};

export function buildFormFieldAriaDescribedBy(
  messageIds: FormFieldMessageIds,
): string | undefined {
  const ids = [
    messageIds.descriptionId,
    messageIds.helperId,
    messageIds.warningId,
    messageIds.errorId,
    messageIds.successId,
  ].filter((id): id is string => Boolean(id));

  return ids.length > 0 ? ids.join(" ") : undefined;
}

export type FormFieldProps = HTMLAttributes<HTMLDivElement>;

export const FormField = forwardRef<HTMLDivElement, FormFieldProps>(
  function FormField({ className, ...props }, ref) {
    return <div className={cn("space-y-2", className)} ref={ref} {...props} />;
  },
);

export type FormLabelProps = HTMLAttributes<HTMLElement> & {
  as?: ElementType;
  children?: ReactNode;
  htmlFor?: string;
};

const FORM_LABEL_CLASS = "block text-sm font-semibold text-on-surface";

export const FormLabel = forwardRef<HTMLElement, FormLabelProps>(
  function FormLabel({ as: Component = "label", className, ...props }, ref) {
    return (
      <Component
        className={cn(FORM_LABEL_CLASS, className)}
        ref={ref}
        {...props}
      />
    );
  },
);

export interface FormDescriptionProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  variant?: "body" | "supporting";
}

const FORM_BODY_TEXT_CLASS = "m-0 text-body-medium text-on-surface";
const FORM_SUPPORTING_TEXT_CLASS =
  "m-0 text-body-small text-on-surface-variant";

export const FormDescription = forwardRef<HTMLElement, FormDescriptionProps>(
  function FormDescription(
    { as: Component = "p", className, variant = "supporting", ...props },
    ref,
  ) {
    return (
      <Component
        className={cn(
          variant === "body"
            ? FORM_BODY_TEXT_CLASS
            : FORM_SUPPORTING_TEXT_CLASS,
          className,
        )}
        ref={ref}
        {...props}
      />
    );
  },
);

export interface FormHelperTextProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
}

export const FormHelperText = forwardRef<HTMLElement, FormHelperTextProps>(
  function FormHelperText({ as: Component = "p", className, ...props }, ref) {
    return (
      <Component
        className={cn(FORM_SUPPORTING_TEXT_CLASS, className)}
        ref={ref}
        {...props}
      />
    );
  },
);

export interface FormErrorProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  role?: "alert" | "status";
}

export const FormError = forwardRef<HTMLElement, FormErrorProps>(
  function FormError(
    { as: Component = "p", className, role = "alert", ...props },
    ref,
  ) {
    return (
      <Component
        className={cn(
          "m-0 text-body-small font-medium text-on-error-container",
          className,
        )}
        ref={ref}
        role={role}
        {...props}
      />
    );
  },
);

export interface FormWarningProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  role?: "alert" | "status";
}

export const FormWarning = forwardRef<HTMLElement, FormWarningProps>(
  function FormWarning(
    { as: Component = "p", className, role, ...props },
    ref,
  ) {
    return (
      <Component
        className={cn(
          "m-0 text-body-small font-medium text-on-warning-container",
          className,
        )}
        ref={ref}
        role={role}
        {...props}
      />
    );
  },
);

export interface FormSuccessProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  role?: "alert" | "status";
}

export const FormSuccess = forwardRef<HTMLElement, FormSuccessProps>(
  function FormSuccess(
    { as: Component = "p", className, role = "status", ...props },
    ref,
  ) {
    return (
      <Component
        className={cn(
          "m-0 text-body-small font-medium text-on-success-container",
          className,
        )}
        ref={ref}
        role={role}
        {...props}
      />
    );
  },
);

export type FormFieldGroupProps = FieldsetHTMLAttributes<HTMLFieldSetElement>;

export const FormFieldGroup = forwardRef<
  HTMLFieldSetElement,
  FormFieldGroupProps
>(function FormFieldGroup({ className, ...props }, ref) {
  return (
    <fieldset
      className={cn("m-0 space-y-2 border-0 p-0", className)}
      ref={ref}
      {...props}
    />
  );
});

export type FormFieldGroupLabelProps = HTMLAttributes<HTMLLegendElement>;

export const FormFieldGroupLabel = forwardRef<
  HTMLLegendElement,
  FormFieldGroupLabelProps
>(function FormFieldGroupLabel({ className, ...props }, ref) {
  return (
    <legend
      className={cn(FORM_LABEL_CLASS, "px-0", className)}
      ref={ref}
      {...props}
    />
  );
});
