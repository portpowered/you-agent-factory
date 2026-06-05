import { cn } from "../../../lib/cn";

const CHOOSE_FILE_SHELL_BASE_CLASS =
  "rounded-xl border border-dashed border-outline-variant bg-surface-container-low transition-colors";

const CHOOSE_FILE_SHELL_DRAG_ACTIVE_CLASS =
  "border-outline-variant bg-af-overlay";

const CHOOSE_FILE_SHELL_DISABLED_CLASS =
  "cursor-not-allowed text-on-surface-disabled";

export interface ChooseFileShellClassNameOptions {
  className?: string;
  disabled?: boolean;
  dragActive?: boolean;
}

export function chooseFileShellClassName({
  className,
  disabled = false,
  dragActive = false,
}: ChooseFileShellClassNameOptions = {}): string {
  return cn(
    CHOOSE_FILE_SHELL_BASE_CLASS,
    dragActive && CHOOSE_FILE_SHELL_DRAG_ACTIVE_CLASS,
    disabled && CHOOSE_FILE_SHELL_DISABLED_CLASS,
    className,
  );
}
