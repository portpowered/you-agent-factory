const ACCENT_CHROME_TOKENS = [
  "bg-af-accent-surface",
  "border-af-accent-border",
  "file:bg-af-accent-surface",
  "file:text-af-accent",
];

const NEUTRAL_SHELL_TOKENS = [
  "border-dashed",
  "border-af-border-strong",
  "bg-af-surface-subtle",
];

export function assertChooseFileShellNeutral(className, label) {
  for (const token of NEUTRAL_SHELL_TOKENS) {
    if (!className.includes(token)) {
      throw new Error(`${label} missing ${token}: ${className}`);
    }
  }

  for (const token of ACCENT_CHROME_TOKENS) {
    if (className.includes(token)) {
      throw new Error(`${label} must not include ${token}: ${className}`);
    }
  }
}

export function assertChooseFileDragActiveNeutral(className, label) {
  if (!className.includes("border-af-border-strong")) {
    throw new Error(
      `${label} drag-active missing border-af-border-strong: ${className}`,
    );
  }

  if (!className.includes("bg-af-overlay")) {
    throw new Error(`${label} drag-active missing bg-af-overlay: ${className}`);
  }

  for (const token of ACCENT_CHROME_TOKENS) {
    if (className.includes(token)) {
      throw new Error(
        `${label} drag-active must not include ${token}: ${className}`,
      );
    }
  }
}
