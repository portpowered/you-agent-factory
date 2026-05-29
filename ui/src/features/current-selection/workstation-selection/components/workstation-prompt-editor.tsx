import Editor, { loader } from "@monaco-editor/react";
import { useEffect, useMemo, useRef } from "react";
import type { RefObject } from "react";
import "monaco-editor/esm/vs/editor/editor.all.js";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import type { editor as MonacoEditorAPI } from "monaco-editor";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import type {
  EditableWorkstationPromptDiagnostic,
  EditableWorkstationPromptHelpState,
} from "../lib/detail-card-types";
import {
  buildWorkstationPromptMarkers,
  isInsideTemplate,
  registerWorkstationPromptCompletionProvider,
  registerWorkstationPromptMonaco,
  WORKSTATION_PROMPT_LANGUAGE_ID,
  WORKSTATION_PROMPT_THEME_ID,
} from "./workstation-prompt-monaco";

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
  quickSuggestions: {
    comments: false,
    other: true,
    strings: true,
  },
  scrollBeyondLastLine: false,
  suggestOnTriggerCharacters: true,
  tabSize: 2,
  wordWrap: "on",
  wordWrapColumn: 80,
  wrappingIndent: "same",
} as const;

let monacoSetupState: "error" | "ready" = "ready";

if (import.meta.env.MODE !== "test") {
  try {
    registerWorkstationPromptMonaco(monaco);
    loader.config({ monaco });
  } catch {
    monacoSetupState = "error";
  }
}

interface WorkstationPromptEditorProps {
  ariaLabel: string;
  ariaDescribedBy?: string;
  ariaInvalid?: boolean;
  autocompleteState: EditableWorkstationPromptHelpState;
  className?: string;
  diagnostics?: EditableWorkstationPromptDiagnostic[];
  hasDiagnostics?: boolean;
  loadingMessage: string;
  onChange: (value: string) => void;
  onMount?: (editorInstance: MonacoEditorAPI.IStandaloneCodeEditor) => void;
  onReadyChange?: (isReady: boolean) => void;
  onScrollChange?: (scrollPosition: { scrollLeft: number; scrollTop: number }) => void;
  startupErrorMessage: string;
  value: string;
}

export function WorkstationPromptEditor({
  ariaLabel,
  ariaDescribedBy,
  ariaInvalid = false,
  autocompleteState,
  className,
  diagnostics = [],
  hasDiagnostics = false,
  loadingMessage,
  onChange,
  onMount,
  onReadyChange,
  onScrollChange,
  startupErrorMessage,
  value,
}: WorkstationPromptEditorProps) {
  const autocompleteStateRef = useRef(autocompleteState);
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
        "overflow-visible rounded-xl border border-af-border bg-transparent",
        hasDiagnostics
          ? "border-af-danger-border focus-within:border-af-danger"
          : undefined,
        className,
      )}
      height="13.5rem"
      language={WORKSTATION_PROMPT_LANGUAGE_ID}
      onChange={(nextValue) => onChange(nextValue ?? "")}
      onMount={createPromptEditorMountHandler({
        autocompleteStateRef,
        editorRef,
        monacoRef,
        onMount,
        onScrollChange,
      })}
      options={{ ...options, ariaLabel }}
      path="inmemory://model/current-selection/workstation-prompt"
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
  onMount,
  onScrollChange,
}: {
  autocompleteStateRef: RefObject<EditableWorkstationPromptHelpState>;
  editorRef: RefObject<MonacoEditorAPI.IStandaloneCodeEditor | null>;
  monacoRef: RefObject<typeof import("monaco-editor") | null>;
  onMount?: (editorInstance: MonacoEditorAPI.IStandaloneCodeEditor) => void;
  onScrollChange?: (scrollPosition: { scrollLeft: number; scrollTop: number }) => void;
}) {
  return (
    editorInstance: MonacoEditorAPI.IStandaloneCodeEditor,
    monacoInstance: typeof import("monaco-editor"),
  ) => {
    editorRef.current = editorInstance;
    monacoRef.current = monacoInstance;

    const completionProvider = registerWorkstationPromptCompletionProvider(
      monacoInstance,
      () => autocompleteStateRef.current,
    );
    editorInstance.addCommand(monacoInstance.KeyCode.Space, () => {
      editorInstance.trigger("workstation-prompt-space", "type", { text: " " });
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
      completionProvider.dispose();
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

function PromptEditorFallbackState({
  ariaDescribedBy,
  ariaInvalid,
  message,
  status,
  value,
}: {
  ariaDescribedBy?: string;
  ariaInvalid: boolean;
  message: string;
  status: "alert" | "status";
  value: string;
}) {
  return (
    <div
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid ? "true" : undefined}
      className="grid min-h-56 gap-2 rounded-xl border border-af-border bg-transparent px-3 py-3"
      data-monaco-editor-fallback="workstation-prompt"
      role={status}
    >
      <p className={cn("m-0 text-af-text-muted", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {message}
      </p>
      <pre
        className={cn(
          "m-0 whitespace-pre-wrap break-words text-af-text-muted [overflow-wrap:anywhere]",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
      >
        {value}
      </pre>
    </div>
  );
}
