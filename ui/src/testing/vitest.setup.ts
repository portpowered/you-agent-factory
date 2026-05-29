/** Vitest-only setup. Bun unit/coverage lanes use `ui/testing/bun-*.preload.ts`. */
import "./app-shell-vitest-mocks";
import { configure } from "@testing-library/react";
import { createElement, type ReactNode, useEffect, useMemo, useState } from "react";
import { vi } from "vitest";

Object.assign(globalThis, {
  IS_REACT_ACT_ENVIRONMENT: true,
});

configure({
  asyncUtilTimeout: 10_000,
});

if (typeof HTMLAnchorElement !== "undefined") {
  const originalAnchorClick = HTMLAnchorElement.prototype.click;

  HTMLAnchorElement.prototype.click = function click(): void {
    if (this.download.length > 0) {
      return;
    }

    originalAnchorClick.call(this);
  };
}

if (typeof document !== "undefined" && !document.queryCommandSupported) {
  document.queryCommandSupported = () => false;
}

vi.mock("@monaco-editor/react", () => ({
  default: ({
    loading,
    onChange,
    onMount,
    options,
    value,
    wrapperProps,
  }: {
    loading?: ReactNode;
    onChange?: (nextValue: string | undefined) => void;
    onMount?: (editorInstance: unknown, monaco: unknown) => void;
    options?: { ariaLabel?: string };
    value?: string;
    wrapperProps?: Record<string, string | undefined>;
  }) => {
    const [markers, setMarkers] = useState<
      Array<{
        endColumn: number;
        endLineNumber: number;
        message: string;
        startColumn: number;
        startLineNumber: number;
      }>
    >([]);
    const model = useMemo(
      () => ({
        __setMarkers: setMarkers,
      }),
      [],
    );

    useEffect(() => {
      const disposeListeners: Array<() => void> = [];
      onMount?.(
        {
          addCommand: () => undefined,
          getModel: () => model,
          getPosition: () => ({ column: 1, lineNumber: 1 }),
          getScrollLeft: () => 0,
          getScrollTop: () => 0,
          onDidDispose: (listener: () => void) => {
            disposeListeners.push(listener);
            return { dispose() {} };
          },
          onDidChangeModelContent: () => ({ dispose() {} }),
          onDidScrollChange: (
            listener: (event: { scrollLeft: number; scrollTop: number }) => void,
          ) => {
            listener({ scrollLeft: 3, scrollTop: 4 });
            return { dispose() {} };
          },
        },
        {
          KeyCode: { Space: 10 },
          editor: {
            setModelMarkers: (
              nextModel: typeof model,
              _owner: string,
              nextMarkers: typeof markers,
            ) => {
              nextModel.__setMarkers(nextMarkers);
            },
          },
          languages: {
            CompletionItemKind: { Variable: 4 },
            CompletionTriggerKind: { Invoke: 0, TriggerCharacter: 1 },
            registerCompletionItemProvider: () => ({
              dispose() {},
            }),
          },
        },
      );

      return () => {
        for (const listener of disposeListeners) {
          listener();
        }
      };
    }, [model, onMount]);

    return createElement(
      "div",
      {
        ...wrapperProps,
        "data-monaco-marker-count": String(markers.length),
        "data-monaco-marker-messages": JSON.stringify(
          markers.map((marker) => marker.message),
        ),
        "data-monaco-marker-ranges": JSON.stringify(markers),
      },
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
    );
  },
  loader: {
    config: vi.fn(),
  },
}));

const mockedMonacoModule = {
  editor: {
    defineTheme: vi.fn(),
    setModelMarkers: vi.fn(),
  },
  languages: {
    CompletionItemKind: { Variable: 4 },
    CompletionTriggerKind: { Invoke: 0, TriggerCharacter: 1 },
    register: vi.fn(),
    registerCompletionItemProvider: vi.fn(() => ({
      dispose() {},
    })),
    setMonarchTokensProvider: vi.fn(),
  },
};

vi.mock("monaco-editor/esm/vs/editor/editor.all.js", () => ({}));
vi.mock("monaco-editor/esm/vs/editor/editor.api.js", () => mockedMonacoModule);
