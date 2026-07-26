import Editor from "@monaco-editor/react";
import type { RefObject } from "react";
import { useEffect, useMemo, useRef } from "react";
import "monaco-editor/esm/vs/editor/editor.all.js";
import type { editor as MonacoEditorAPI } from "monaco-editor";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import { cn } from "../../lib/cn";
import { Code, Text } from "@you-agent-factory/components/primitives";
import {
  applyWorkstationPromptTheme,
  buildWorkstationPromptMarkers,
  isInsideTemplate,
  registerWorkstationPromptCompletionProvider,
  registerWorkstationPromptMonaco,
  WORKSTATION_PROMPT_LANGUAGE_ID,
  WORKSTATION_PROMPT_THEME_ID,
} from "./monaco-prompt-setup";
import { configureMonacoReactLoader } from "./monaco-react-loader";
import type {
  PromptEditorAutocompleteState,
  PromptEditorDiagnostic,
} from "./prompt-editor-types";

const PROMPT_EDITOR_OPTIONS = {
  automaticLayout: true,
  fontFamily: "inherit",
  fontLigatures: false,
  fontSize: 14,
  fixedOverflowWidgets: true,
  glyphMargin: false,
  lineDecorationsWidth: 10,
  lineNumbers: "off",
  lineNumbersMinChars: 0,
  minimap: { enabled: false },
  overviewRulerBorder: false,
  overviewRulerLanes: 0,
  padding: { bottom: 12, top: 12 },
  renderFinalNewline: "off",
  renderLineHighlight: "none",
  roundedSelection: false,
  scrollbar: {
    alwaysConsumeMouseWheel: false,
    horizontalScrollbarSize: 8,
    useShadows: false,
    verticalScrollbarSize: 8,
  },
  quickSuggestions: false,
  scrollBeyondLastLine: false,
  suggestOnTriggerCharacters: true,
  tabSize: 2,
  wordWrap: "on",
  wordWrapColumn: 80,
  wrappingIndent: "same",
} as const;

export const CURRENT_SELECTION_WORKSTATION_PROMPT_MODEL_PATH =
  "inmemory://model/current-selection/workstation-prompt";
export const FACTORY_GRAPH_ADD_WORKSTATION_PROMPT_MODEL_PATH =
  "inmemory://model/factory-graph-add/workstation-prompt";

let monacoSetupState: "error" | "ready" = "ready";

if (import.meta.env.MODE !== "test") {
  try {
    registerWorkstationPromptMonaco(monaco);
    configureMonacoReactLoader(monaco);
  } catch {
    monacoSetupState = "error";
  }
}

interface MonacoPromptEditorProps {
  ariaLabel: string;
  ariaDescribedBy?: string;
  ariaInvalid?: boolean;
  autocompleteState: PromptEditorAutocompleteState;
  className?: string;
  diagnostics?: PromptEditorDiagnostic[];
  hasDiagnostics?: boolean;
  height?: string;
  loadingMessage: string;
  modelPath: string;
  onChange: (value: string) => void;
  onMount?: (editorInstance: MonacoEditorAPI.IStandaloneCodeEditor) => void;
  onReadyChange?: (isReady: boolean) => void;
  onScrollChange?: (scrollPosition: {
    scrollLeft: number;
    scrollTop: number;
  }) => void;
  startupErrorMessage: string;
  value: string;
}

export function MonacoPromptEditor({
  ariaLabel,
  ariaDescribedBy,
  ariaInvalid = false,
  autocompleteState,
  className,
  diagnostics = [],
  hasDiagnostics = false,
  height = "13.5rem",
  loadingMessage,
  modelPath,
  onChange,
  onMount,
  onReadyChange,
  onScrollChange,
  startupErrorMessage,
  value,
}: MonacoPromptEditorProps) {
  const autocompleteStateRef = useRef(autocompleteState);
  const onChangeRef = useRef(onChange);
  const editorRef = useRef<MonacoEditorAPI.IStandaloneCodeEditor | null>(null);
  const monacoRef = useRef<typeof import("monaco-editor") | null>(null);
  const startupState = monacoSetupState;
  const markers = useMemo(
    () => buildWorkstationPromptMarkers(value, diagnostics),
    [diagnostics, value],
  );

  useEffect(() => {
    autocompleteStateRef.current = autocompleteState;
  }, [autocompleteState]);

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    onReadyChange?.(startupState === "ready");

    return () => {
      onReadyChange?.(false);
    };
  }, [onReadyChange]);

  const options = useMemo(() => PROMPT_EDITOR_OPTIONS, []);

  useEffect(() => {
    const editorInstance = editorRef.current;
    const monaco = monacoRef.current;
    const model = editorInstance?.getModel();
    if (!monaco || !model) {
      return;
    }

    monaco.editor.setModelMarkers(
      model,
      "workstation-prompt-validation",
      markers,
    );

    return () => {
      monaco.editor.setModelMarkers(model, "workstation-prompt-validation", []);
    };
  }, [markers]);

  if (startupState !== "ready") {
    return (
      <PromptEditorFallbackState
        ariaDescribedBy={ariaDescribedBy}
        ariaInvalid={ariaInvalid}
        height={height}
        message={
          startupState === "error" ? startupErrorMessage : loadingMessage
        }
        status={startupState === "error" ? "alert" : "status"}
        value={value}
      />
    );
  }

  return (
    <Editor
      className={cn(
        "overflow-visible rounded-xl border border-outline bg-transparent",
        hasDiagnostics
          ? "border-af-danger-border focus-within:border-af-danger"
          : undefined,
        className,
      )}
      height={height}
      language={WORKSTATION_PROMPT_LANGUAGE_ID}
      onChange={(nextValue) => onChange(nextValue ?? "")}
      onMount={createPromptEditorMountHandler({
        autocompleteStateRef,
        editorRef,
        monacoRef,
        onChangeRef,
        onMount,
        onScrollChange,
      })}
      options={{ ...options, ariaLabel }}
      path={modelPath}
      theme={WORKSTATION_PROMPT_THEME_ID}
      value={value}
      width="100%"
      wrapperProps={{
        "aria-describedby": ariaDescribedBy,
        "aria-invalid": ariaInvalid ? "true" : undefined,
        "data-monaco-editor": "workstation-prompt",
      }}
    />
  );
}

function createPromptEditorMountHandler({
  autocompleteStateRef,
  editorRef,
  monacoRef,
  onChangeRef,
  onMount,
  onScrollChange,
}: {
  autocompleteStateRef: RefObject<PromptEditorAutocompleteState>;
  editorRef: RefObject<MonacoEditorAPI.IStandaloneCodeEditor | null>;
  monacoRef: RefObject<typeof import("monaco-editor") | null>;
  onChangeRef: RefObject<(value: string) => void>;
  onMount?: (editorInstance: MonacoEditorAPI.IStandaloneCodeEditor) => void;
  onScrollChange?: (scrollPosition: {
    scrollLeft: number;
    scrollTop: number;
  }) => void;
}) {
  return (
    editorInstance: MonacoEditorAPI.IStandaloneCodeEditor,
    monacoInstance: typeof import("monaco-editor"),
  ) => {
    editorRef.current = editorInstance;
    monacoRef.current = monacoInstance;
    const themeObserver =
      typeof MutationObserver === "undefined" || typeof document === "undefined"
        ? null
        : createPromptThemeObserver(monacoInstance, document.documentElement);

    const completionProvider = registerWorkstationPromptCompletionProvider(
      monacoInstance,
      () => autocompleteStateRef.current,
    );
    editorInstance.addCommand(monacoInstance.KeyCode.Space, () => {
      editorInstance.trigger("workstation-prompt-space", "type", { text: " " });
    });
    const contentChangeListener = editorInstance.onDidChangeModelContent(() => {
      onChangeRef.current(editorInstance.getValue());
    });
    const typeListener = editorInstance.onDidChangeModelContent((event) => {
      const insertedText = event.changes.map((change) => change.text).join("");
      const model = editorInstance.getModel();
      const position = editorInstance.getPosition();
      if (!model || !position || insertedText.length === 0) {
        return;
      }

      const prompt = model.getValue();
      const cursorOffset = model.getOffsetAt(position);
      if (!isInsideTemplate(prompt, cursorOffset)) {
        return;
      }

      const typedTrigger =
        insertedText.includes("{") ||
        insertedText.includes(".") ||
        insertedText.includes("$") ||
        insertedText.includes("(");
      const typedIdentifierText = insertedText.trim().length > 0;
      if (!typedTrigger && !typedIdentifierText) {
        return;
      }

      editorInstance.trigger(
        "workstation-prompt-autocomplete",
        "editor.action.triggerSuggest",
        {},
      );
    });

    editorInstance.onDidDispose(() => {
      editorRef.current = null;
      monacoRef.current = null;
      themeObserver?.disconnect();
      completionProvider.dispose();
      contentChangeListener.dispose();
      typeListener.dispose();
    });
    onMount?.(editorInstance);
    onScrollChange?.({
      scrollLeft: editorInstance.getScrollLeft(),
      scrollTop: editorInstance.getScrollTop(),
    });
    editorInstance.onDidScrollChange((event) => {
      onScrollChange?.({
        scrollLeft: event.scrollLeft,
        scrollTop: event.scrollTop,
      });
    });
  };
}

function createPromptThemeObserver(
  monaco: typeof import("monaco-editor"),
  root: HTMLElement,
) {
  const applyTheme = () => {
    applyWorkstationPromptTheme(monaco, root);
  };

  applyTheme();

  const observer = new MutationObserver((mutations) => {
    if (
      mutations.some(
        (mutation) =>
          mutation.type === "attributes" &&
          mutation.attributeName === "data-color-palette",
      )
    ) {
      applyTheme();
    }
  });

  observer.observe(root, {
    attributeFilter: ["data-color-palette"],
    attributes: true,
  });

  return observer;
}

function PromptEditorFallbackState({
  ariaDescribedBy,
  ariaInvalid,
  height,
  message,
  status,
  value,
}: {
  ariaDescribedBy?: string;
  ariaInvalid: boolean;
  height: string;
  message: string;
  status: "alert" | "status";
  value: string;
}) {
  return (
    <div
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid ? "true" : undefined}
      className="grid h-full min-h-0 gap-2 overflow-auto rounded-xl border border-outline bg-transparent px-3 py-3"
      data-monaco-editor-fallback="workstation-prompt"
      role={status}
      style={{ height }}
    >
      <Text className="m-0 text-on-surface-variant" variant="supporting">
        {message}
      </Text>
      <Code
        as="pre"
        className="m-0 whitespace-pre-wrap break-words text-on-surface-variant [overflow-wrap:anywhere]"
      >
        {value}
      </Code>
    </div>
  );
}
