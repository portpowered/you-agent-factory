import {
  cloneElement,
  isValidElement,
  type ReactElement,
  type ReactNode,
} from "react";

import { cn } from "../../../lib/cn";
import {
  type ChooseFileShellClassNameOptions,
  chooseFileShellClassName,
} from "../lib/choose-file-shell";

const CHOOSE_FILE_FIELD_GROUP_CLASS = "space-y-2";

export interface ChooseFileFieldProps extends ChooseFileShellClassNameOptions {
  afterControl?: ReactNode;
  control: ReactElement<{ className?: string }>;
  description?: ReactNode;
  fieldClassName?: string;
  label?: ReactNode;
}

export function ChooseFileField({
  afterControl,
  className,
  control,
  description,
  disabled = false,
  dragActive = false,
  fieldClassName,
  label,
}: ChooseFileFieldProps) {
  if (!isValidElement(control)) {
    throw new Error("ChooseFileField requires a single React element control.");
  }

  const shellClassName = chooseFileShellClassName({
    className,
    disabled,
    dragActive,
  });
  const mergedControl = cloneElement(control, {
    className: cn(shellClassName, control.props.className),
  });

  return (
    <div className={cn(CHOOSE_FILE_FIELD_GROUP_CLASS, fieldClassName)}>
      {label}
      {mergedControl}
      {description}
      {afterControl}
    </div>
  );
}
