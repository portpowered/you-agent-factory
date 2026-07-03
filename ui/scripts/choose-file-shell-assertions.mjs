const ACCENT_CHROME_TOKENS = [
  "bg-af-accent-surface",
  "border-af-accent-border",
  "file:bg-af-accent-surface",
  "file:text-af-accent",
];

const NEUTRAL_SHELL_TOKENS = [
  "border-dashed",
  "border-outline-variant",
  "bg-surface-container-low",
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
  if (!className.includes("border-outline-variant")) {
    throw new Error(
      `${label} drag-active missing border-outline-variant: ${className}`,
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
