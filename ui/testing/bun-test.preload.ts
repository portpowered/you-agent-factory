/**
 * Bun unit/coverage lane: Testing Library config, jest-dom matchers, DOM shims, and Monaco mocks.
 * Loaded after bun-dom.preload.ts so document/window exist before @testing-library/react imports.
 */
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  mock,
  test,
} from "bun:test";
import { vi } from "vitest";

const isNodeLane = process.env.BUN_TEST_NODE_LANE === "1";

Object.assign(globalThis, {
  describe,
  it,
  test,
  expect,
  beforeEach,
  afterEach,
  vi,
  ...(isNodeLane ? {} : { mock }),
});

if (isNodeLane) {
  // Node lane: vitest `vi` + bun runner globals only (no jsdom, Monaco, or Testing Library).
} else {
  const { configure, cleanup } = await import("@testing-library/react");
  const { createElement, useEffect, useMemo, useState } = await import("react");
  type ReactNode = import("react").ReactNode;
  await import("@testing-library/jest-dom");
  const { installDashboardBrowserTestShims } = await import(
    "../src/components/dashboard/test-browser-shims"
  );

  // Dashboard measurement shims are installed once for the Bun worker process.
  installDashboardBrowserTestShims();

  Object.assign(globalThis, {
    IS_REACT_ACT_ENVIRONMENT: true,
  });

  configure({
    asyncUtilTimeout: 10_000,
  });

  afterEach(() => {
    cleanup();
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

  const monacoLoaderConfig = mock(() => undefined);

  mock.module("@monaco-editor/react", () => ({
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
      config: monacoLoaderConfig,
    },
  }));

  const mockedMonacoModule = {
    editor: {
      defineTheme: mock(() => undefined),
      setModelMarkers: mock(() => undefined),
    },
    languages: {
      CompletionItemKind: { Variable: 4 },
      CompletionTriggerKind: { Invoke: 0, TriggerCharacter: 1 },
      register: mock(() => undefined),
      registerCompletionItemProvider: mock(() => ({
        dispose() {},
      })),
      setMonarchTokensProvider: mock(() => undefined),
    },
  };

  mock.module("monaco-editor/esm/vs/editor/editor.all.js", () => ({}));
  mock.module("monaco-editor/esm/vs/editor/editor.api.js", () => mockedMonacoModule);
}
