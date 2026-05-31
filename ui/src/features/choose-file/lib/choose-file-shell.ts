import { cn } from "../../../lib/cn";

export const CHOOSE_FILE_FIELD_GROUP_CLASS = "space-y-2";

const CHOOSE_FILE_SHELL_BASE_CLASS =
  "rounded-xl border border-dashed border-af-border-strong bg-af-surface-subtle transition-colors";

const CHOOSE_FILE_SHELL_DRAG_ACTIVE_CLASS =
  "border-af-border-strong bg-af-overlay";

const CHOOSE_FILE_SHELL_DISABLED_CLASS =
  "cursor-not-allowed text-af-text-disabled";

/** Classes for native `<input type="file">` chrome inside the dashed shell. */
export const CHOOSE_FILE_NATIVE_INPUT_CLASS =
  "block w-full px-3 py-3 text-sm text-af-text-muted file:mr-3 file:rounded-lg file:border-0 file:bg-af-surface-raised file:px-3 file:py-2 file:text-sm file:font-semibold file:text-af-text hover:bg-af-overlay";

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
