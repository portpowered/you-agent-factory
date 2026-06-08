import {
  type ElementType,
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../../lib/cn";
import { DashboardText } from "./dashboard-typography-components";

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

export const FormLabel = forwardRef<HTMLElement, FormLabelProps>(
  function FormLabel({ as: Component = "label", className, ...props }, ref) {
    return (
      <Component
        className={cn("block text-sm font-semibold text-on-surface", className)}
        ref={ref}
        {...props}
      />
    );
  },
);

export interface FormDescriptionProps extends HTMLAttributes<HTMLElement> {
  variant?: "body" | "supporting";
}

export const FormDescription = forwardRef<HTMLElement, FormDescriptionProps>(
  function FormDescription(
    { className, variant = "supporting", ...props },
    ref,
  ) {
    return (
      <DashboardText
        className={cn("m-0", className)}
        ref={ref}
        variant={variant}
        {...props}
      />
    );
  },
);

export interface FormErrorProps extends HTMLAttributes<HTMLElement> {
  role?: "alert" | "status";
}

export const FormError = forwardRef<HTMLElement, FormErrorProps>(
  function FormError({ className, role = "alert", ...props }, ref) {
    return (
      <DashboardText
        className={cn("m-0 font-medium text-on-error-container", className)}
        ref={ref}
        role={role}
        variant="supporting"
        {...props}
      />
    );
  },
);

export interface FormWarningProps extends HTMLAttributes<HTMLElement> {
  role?: "alert" | "status";
}

export const FormWarning = forwardRef<HTMLElement, FormWarningProps>(
  function FormWarning({ className, ...props }, ref) {
    return (
      <DashboardText
        className={cn("m-0 font-medium text-on-warning-container", className)}
        ref={ref}
        variant="supporting"
        {...props}
      />
    );
  },
);
