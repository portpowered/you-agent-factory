import Editor from "@monaco-editor/react";
import type { RefObject } from "react";
import { useEffect, useMemo, useRef } from "react";
import "monaco-editor/esm/vs/editor/editor.all.js";
import type { editor as MonacoEditorAPI } from "monaco-editor";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import { cn } from "../../lib/cn";
import { Text, Textarea } from "../ui";
import {
  applyWorkstationPromptTheme,
  registerWorkstationPromptMonaco,
  WORKSTATION_PROMPT_THEME_ID,
} from "./monaco-prompt-setup";
import { configureMonacoReactLoader } from "./monaco-react-loader";

const TEXT_EDITOR_OPTIONS = {
  automaticLayout: true,
  fontFamily: "inherit",
  fontLigatures: false,
  fontSize: 14,
  fixedOverflowWidgets: true,
  glyphMargin: false,
  lineDecorationsWidth: 10,
  lineNumbers: "on",
  lineNumbersMinChars: 3,
  minimap: { enabled: false },
  overviewRulerBorder: false,
  overviewRulerLanes: 0,
  padding: { bottom: 12, top: 12 },
  renderFinalNewline: "off",
  renderLineHighlight: "line",
  roundedSelection: false,
  scrollbar: {
    alwaysConsumeMouseWheel: false,
    horizontalScrollbarSize: 8,
    useShadows: false,
    verticalScrollbarSize: 8,
  },
  quickSuggestions: false,
  scrollBeyondLastLine: false,
  tabSize: 2,
  wordWrap: "on",
  wordWrapColumn: 80,
  wrappingIndent: "same",
} as const;

export const CURRENT_SELECTION_FACTORY_DOC_MODEL_PATH =
  "inmemory://model/current-selection/factory-doc";

let monacoSetupState: "error" | "ready" = "ready";

if (import.meta.env.MODE !== "test") {
  try {
    registerWorkstationPromptMonaco(monaco);
    configureMonacoReactLoader(monaco);
  } catch {
    monacoSetupState = "error";
  }
}

interface MonacoTextEditorProps {
  ariaDescribedBy?: string;
  ariaInvalid?: boolean;
  ariaLabel: string;
  className?: string;
  hasError?: boolean;
  height?: string;
  id?: string;
  loadingMessage: string;
  modelPath: string;
  onChange: (value: string) => void;
  startupErrorMessage: string;
  value: string;
}

export function MonacoTextEditor({
  ariaDescribedBy,
  ariaInvalid = false,
  ariaLabel,
  className,
  hasError = false,
  height = "16rem",
  id,
  loadingMessage,
  modelPath,
  onChange,
  startupErrorMessage,
  value,
}: MonacoTextEditorProps) {
  const onChangeRef = useRef(onChange);
  const editorRef = useRef<MonacoEditorAPI.IStandaloneCodeEditor | null>(null);
  const startupState = monacoSetupState;
  const options = useMemo(() => TEXT_EDITOR_OPTIONS, []);

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    const editorInstance = editorRef.current;
    const model = editorInstance?.getModel();
    if (!editorInstance || !model || model.getValue() === value) {
      return;
    }

    if (
      typeof editorInstance.getSelection !== "function" ||
      typeof editorInstance.getScrollTop !== "function" ||
      typeof editorInstance.getScrollLeft !== "function" ||
      typeof editorInstance.setScrollPosition !== "function" ||
      typeof model.pushEditOperations !== "function"
    ) {
      if (typeof model.setValue === "function") {
        model.setValue(value);
      }
      return;
    }

    const selection = editorInstance.getSelection();
    const scrollTop = editorInstance.getScrollTop();
    const scrollLeft = editorInstance.getScrollLeft();

    model.pushEditOperations(
      [],
      [
        {
          range: model.getFullModelRange(),
          text: value,
        },
      ],
      () => (selection ? [selection] : null),
    );
    if (selection) {
      editorInstance.setSelection(selection);
    }
    editorInstance.setScrollPosition({ scrollLeft, scrollTop });
  }, [value]);

  if (startupState !== "ready") {
    return (
      <TextEditorFallbackState
        ariaDescribedBy={ariaDescribedBy}
        ariaInvalid={ariaInvalid}
        ariaLabel={ariaLabel}
        height={height}
        id={id}
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
        "overflow-visible rounded-lg border border-outline bg-transparent",
        hasError
          ? "border-af-danger-border focus-within:border-af-danger"
          : undefined,
        className,
      )}
      height={height}
      defaultValue={value}
      language="plaintext"
      onMount={createTextEditorMountHandler({ editorRef, onChangeRef })}
      options={{ ...options, ariaLabel }}
      path={modelPath}
      theme={WORKSTATION_PROMPT_THEME_ID}
      width="100%"
      wrapperProps={{
        "aria-describedby": ariaDescribedBy,
        "aria-invalid": ariaInvalid ? "true" : undefined,
        "data-monaco-editor": "factory-doc-text",
        id,
      }}
    />
  );
}

function createTextEditorMountHandler({
  editorRef,
  onChangeRef,
}: {
  editorRef: RefObject<MonacoEditorAPI.IStandaloneCodeEditor | null>;
  onChangeRef: RefObject<(value: string) => void>;
}) {
  return (editorInstance: MonacoEditorAPI.IStandaloneCodeEditor) => {
    editorRef.current = editorInstance;
    const themeObserver =
      typeof MutationObserver === "undefined" || typeof document === "undefined"
        ? null
        : createTextEditorThemeObserver(document.documentElement);
    const contentChangeListener = editorInstance.onDidChangeModelContent(() => {
      onChangeRef.current(editorInstance.getValue());
    });

    editorInstance.onDidDispose(() => {
      editorRef.current = null;
      themeObserver?.disconnect();
      contentChangeListener.dispose();
    });
  };
}

function createTextEditorThemeObserver(root: HTMLElement) {
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

function TextEditorFallbackState({
  ariaDescribedBy,
  ariaInvalid,
  ariaLabel,
  height,
  id,
  message,
  status,
  value,
}: {
  ariaDescribedBy?: string;
  ariaInvalid: boolean;
  ariaLabel: string;
  height: string;
  id?: string;
  message: string;
  status: "alert" | "status";
  value: string;
}) {
  return (
    <div
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid ? "true" : undefined}
      className="grid min-h-0 gap-1 overflow-auto rounded-lg border border-outline bg-transparent px-3 py-2"
      data-monaco-editor-fallback="factory-doc-text"
      id={id}
      role={status}
      style={{ height }}
    >
      <Text className="m-0 text-on-surface-variant" variant="supporting">
        {message}
      </Text>
      <Textarea
        aria-label={ariaLabel}
        defaultValue={value}
        readOnly
        variant="plain"
      />
    </div>
  );
}
