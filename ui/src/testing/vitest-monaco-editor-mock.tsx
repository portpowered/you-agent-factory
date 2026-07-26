import {
  createElement,
  type Dispatch,
  type ReactNode,
  type RefObject,
  type SetStateAction,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

type MonacoEditorMockProps = {
  className?: string;
  loading?: ReactNode;
  onChange?: (nextValue: string | undefined) => void;
  onMount?: (editorInstance: unknown, monaco: unknown) => void;
  options?: { ariaLabel?: string };
  value?: string;
  wrapperProps?: Record<string, string | undefined>;
};

type MonacoMarkers = Array<{
  endColumn: number;
  endLineNumber: number;
  message: string;
  startColumn: number;
  startLineNumber: number;
}>;

type MonacoThemeApplications = Array<{ base: string | null; name: string }>;

export function MonacoEditorMock({
  className,
  loading,
  onChange,
  onMount,
  options,
  value,
  wrapperProps,
}: MonacoEditorMockProps) {
  const [markers, setMarkers] = useState<MonacoMarkers>([]);
  const [themeApplications, setThemeApplications] =
    useState<MonacoThemeApplications>([]);
  const [appliedThemeNames, setAppliedThemeNames] = useState<string[]>([]);
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
    contentChangeListenersRef.current = [];
    onMount?.(
      createMockEditorInstance({
        contentChangeListenersRef,
        disposeListeners,
        editorValueRef,
        model,
      }),
      createMockMonacoModule({
        model,
        setAppliedThemeNames,
        setThemeApplications,
      }),
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
    buildWrapperProps({
      appliedThemeNames,
      className,
      markers,
      themeApplications,
      wrapperProps,
    }),
    loading
      ? createElement("div", { "data-monaco-loading": "true" }, loading)
      : null,
    createEditorTextarea({
      editorValueRef,
      notifyContentChange,
      onChange,
      options,
      value,
      wrapperProps,
    }),
  );
}

function createMockEditorInstance({
  contentChangeListenersRef,
  disposeListeners,
  editorValueRef,
  model,
}: {
  contentChangeListenersRef: RefObject<
    Array<(event: { changes: Array<{ text: string }> }) => void>
  >;
  disposeListeners: Array<() => void>;
  editorValueRef: RefObject<string>;
  model: {
    __setMarkers: Dispatch<SetStateAction<MonacoMarkers>>;
    getOffsetAt: () => number;
    getValue: () => string;
  };
}) {
  return {
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
      listener: (event: { scrollLeft: number; scrollTop: number }) => void,
    ) => {
      listener({ scrollLeft: 3, scrollTop: 4 });
      return { dispose() {} };
    },
    trigger: () => undefined,
  };
}

function createMockMonacoModule({
  model,
  setAppliedThemeNames,
  setThemeApplications,
}: {
  model: {
    __setMarkers: Dispatch<SetStateAction<MonacoMarkers>>;
    getOffsetAt: () => number;
    getValue: () => string;
  };
  setAppliedThemeNames: Dispatch<SetStateAction<string[]>>;
  setThemeApplications: Dispatch<SetStateAction<MonacoThemeApplications>>;
}) {
  return {
    KeyCode: { Space: 10 },
    editor: {
      defineTheme: (name: string, themeData: { base?: string }) => {
        setThemeApplications((current) => [
          ...current,
          { base: themeData.base ?? null, name },
        ]);
      },
      setModelMarkers: (
        nextModel: typeof model,
        _owner: string,
        nextMarkers: MonacoMarkers,
      ) => {
        nextModel.__setMarkers(nextMarkers);
      },
      setTheme: (name: string) => {
        setAppliedThemeNames((current) => [...current, name]);
      },
    },
    languages: {
      CompletionItemKind: { Field: 13, Variable: 4 },
      CompletionTriggerKind: { Invoke: 0, TriggerCharacter: 1 },
      registerCompletionItemProvider: () => ({
        dispose() {},
      }),
    },
  };
}

function buildWrapperProps({
  appliedThemeNames,
  className,
  markers,
  themeApplications,
  wrapperProps,
}: {
  appliedThemeNames: string[];
  className?: string;
  markers: MonacoMarkers;
  themeApplications: MonacoThemeApplications;
  wrapperProps?: Record<string, string | undefined>;
}) {
  return {
    ...wrapperProps,
    className,
    "data-monaco-marker-count": String(markers.length),
    "data-monaco-marker-messages": JSON.stringify(
      markers.map((marker) => marker.message),
    ),
    "data-monaco-marker-ranges": JSON.stringify(markers),
    "data-monaco-theme-application-count": String(themeApplications.length),
    "data-monaco-theme-bases": JSON.stringify(
      themeApplications.map((application) => application.base),
    ),
    "data-monaco-theme-set-count": String(appliedThemeNames.length),
    "data-monaco-theme-set-names": JSON.stringify(appliedThemeNames),
  };
}

function createEditorTextarea({
  editorValueRef,
  notifyContentChange,
  onChange,
  options,
  value,
  wrapperProps,
}: {
  editorValueRef: RefObject<string>;
  notifyContentChange: (nextValue: string, insertedText: string) => void;
  onChange?: (nextValue: string | undefined) => void;
  options?: { ariaLabel?: string };
  value?: string;
  wrapperProps?: Record<string, string | undefined>;
}) {
  return createElement("textarea", {
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
  });
}
