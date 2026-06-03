import "./guarded-suite-console.setup";

import { configure } from "@testing-library/react";
import { installResizeObserverShim } from "../components/dashboard/test-browser-shims";

installResizeObserverShim();

import {
  createElement,
  type ReactNode,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
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
    className,
    loading,
    onChange,
    onMount,
    options,
    value,
    wrapperProps,
  }: {
    className?: string;
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
    const editorValueRef = useRef(value ?? "");
    editorValueRef.current = value ?? "";
    const contentChangeListenersRef = useRef<
      Array<(event: { changes: Array<{ text: string }> }) => void>
    >([]);
    const model = useMemo(
      () => ({
        __setMarkers: setMarkers,
        getOffsetAt: () => editorValueRef.current.length,
        getValue: () => editorValueRef.current,
      }),
      [],
    );

    useEffect(() => {
      const disposeListeners: Array<() => void> = [];
      const triggerSuggest = vi.fn();
      contentChangeListenersRef.current = [];
      onMount?.(
        {
          addCommand: () => undefined,
          getModel: () => model,
          getPosition: () => ({ column: 1, lineNumber: 1 }),
          getScrollLeft: () => 0,
          getScrollTop: () => 0,
          getValue: () => editorValueRef.current,
          onDidDispose: (listener: () => void) => {
            disposeListeners.push(listener);
            return { dispose() {} };
          },
          onDidChangeModelContent: (
            listener: (event: { changes: Array<{ text: string }> }) => void,
          ) => {
            contentChangeListenersRef.current.push(listener);
            return {
              dispose() {
                contentChangeListenersRef.current =
                  contentChangeListenersRef.current.filter(
                    (registeredListener) => registeredListener !== listener,
                  );
              },
            };
          },
          onDidScrollChange: (
            listener: (event: {
              scrollLeft: number;
              scrollTop: number;
            }) => void,
          ) => {
            listener({ scrollLeft: 3, scrollTop: 4 });
            return { dispose() {} };
          },
          trigger: triggerSuggest,
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
            CompletionItemKind: { Field: 13, Variable: 4 },
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

    const notifyContentChange = (nextValue: string, insertedText: string) => {
      editorValueRef.current = nextValue;
      const event = { changes: [{ text: insertedText }] };
      for (const listener of contentChangeListenersRef.current) {
        listener(event);
      }
    };

    return createElement(
      "div",
      {
        ...wrapperProps,
        className,
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
        "data-monaco-editor":
          wrapperProps?.["data-monaco-editor"] ?? "workstation-prompt",
        onChange: (event: Event) => {
          const nextValue = (event.target as HTMLTextAreaElement).value;
          const insertedText = nextValue.slice(editorValueRef.current.length);
          notifyContentChange(nextValue, insertedText);
          onChange?.(nextValue);
        },
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
