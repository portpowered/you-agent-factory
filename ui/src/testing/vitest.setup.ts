import { createElement, type ReactNode } from "react";
import { vi } from "vitest";

if (typeof HTMLAnchorElement !== "undefined") {
  const originalAnchorClick = HTMLAnchorElement.prototype.click;

  HTMLAnchorElement.prototype.click = function click(): void {
    if (this.download.length > 0) {
      return;
    }

    originalAnchorClick.call(this);
  };
}

vi.mock("@monaco-editor/react", () => ({
  default: ({
    loading,
    onChange,
    options,
    value,
    wrapperProps,
  }: {
    loading?: ReactNode;
    onChange?: (nextValue: string | undefined) => void;
    options?: { ariaLabel?: string };
    value?: string;
    wrapperProps?: Record<string, string | undefined>;
  }) =>
    createElement(
      "div",
      wrapperProps,
      loading
        ? createElement("div", { "data-monaco-loading": "true" }, loading)
        : null,
      createElement("textarea", {
        "aria-label": options?.ariaLabel,
        "data-monaco-editor": "workstation-prompt",
        onChange: (event: Event) =>
          onChange?.((event.target as HTMLTextAreaElement).value),
        value: value ?? "",
      }),
    ),
  loader: {
    config: vi.fn(),
  },
}));
