import Editor, { loader } from "@monaco-editor/react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { editor as MonacoEditorAPI } from "monaco-editor";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import type {
  EditableWorkstationPromptDiagnostic,
  EditableWorkstationPromptHelpState,
} from "../detail-card-types";
import {
  buildWorkstationPromptMarkers,
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
  scrollBeyondLastLine: false,
  tabSize: 2,
  wordWrap: "on",
  wordWrapColumn: 80,
  wrappingIndent: "same",
} as const;

let monacoLoaderReady = import.meta.env.MODE === "test";
let monacoLoaderPromise: Promise<void> | null = null;

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
  const [startupState, setStartupState] = useState<"error" | "loading" | "ready">(
    monacoLoaderReady ? "ready" : "loading",
  );
  const markers = useMemo(
    () => buildWorkstationPromptMarkers(value, diagnostics),
    [diagnostics, value],
  );

  useEffect(() => {
    autocompleteStateRef.current = autocompleteState;
  }, [autocompleteState]);

  useEffect(() => {
    let cancelled = false;

    onReadyChange?.(startupState === "ready");

    if (startupState === "ready" || import.meta.env.MODE === "test") {
      return () => {
        onReadyChange?.(false);
      };
    }

    configureMonacoLoader()
      .then(() => {
        if (!cancelled) {
          setStartupState("ready");
        }
      })
      .catch(() => {
        if (!cancelled) {
          setStartupState("error");
        }
      });

    return () => {
      cancelled = true;
      onReadyChange?.(false);
    };
  }, [onReadyChange, startupState]);

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
        "overflow-hidden rounded-xl border border-af-overlay/14 bg-transparent",
        hasDiagnostics
          ? "border-af-danger/45 focus-within:border-af-danger"
          : undefined,
        className,
      )}
      defaultLanguage={WORKSTATION_PROMPT_LANGUAGE_ID}
      height="13.5rem"
      onChange={(nextValue) => onChange(nextValue ?? "")}
      onMount={(editorInstance, monaco) => {
        editorRef.current = editorInstance;
        monacoRef.current = monaco;
        const completionProvider = registerWorkstationPromptCompletionProvider(
          monaco,
          () => autocompleteStateRef.current,
        );

        editorInstance.onDidDispose(() => {
          editorRef.current = null;
          monacoRef.current = null;
          completionProvider.dispose();
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
      }}
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

async function configureMonacoLoader() {
  if (monacoLoaderReady) {
    return;
  }

  if (!monacoLoaderPromise) {
    monacoLoaderPromise = Promise.all([
      import("monaco-editor/esm/vs/editor/editor.api.js"),
      import("monaco-editor/esm/vs/editor/editor.worker?worker&inline"),
    ]).then(([monaco, editorWorker]) => {
        registerWorkstationPromptMonaco(monaco);

        self.MonacoEnvironment = {
          getWorker() {
            return new editorWorker.default();
          },
        };

        loader.config({ monaco });
        monacoLoaderReady = true;
      });
  }

  return monacoLoaderPromise;
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
      className="grid min-h-56 gap-2 rounded-xl border border-af-overlay/14 bg-transparent px-3 py-3"
      data-monaco-editor-fallback="workstation-prompt"
      role={status}
    >
      <p className={cn("m-0 text-af-ink/72", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {message}
      </p>
      <pre
        className={cn(
          "m-0 whitespace-pre-wrap break-words text-af-ink/78 [overflow-wrap:anywhere]",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
      >
        {value}
      </pre>
    </div>
  );
}
