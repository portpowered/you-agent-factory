import Editor from "@monaco-editor/react";
import type { RefObject } from "react";
import { useEffect, useMemo, useRef } from "react";
import "monaco-editor/esm/vs/editor/editor.all.js";
import type { editor as MonacoEditorAPI } from "monaco-editor";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import { cn } from "../../lib/cn";
import { Text, Textarea } from "../ui";
import {
  applyWorkstationGuardSelectorTheme,
  registerWorkstationGuardSelectorCompletionProvider,
  registerWorkstationGuardSelectorMonaco,
  WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID,
  WORKSTATION_GUARD_SELECTOR_THEME_ID,
} from "./monaco-guard-selector-setup";
import { configureMonacoReactLoader } from "./monaco-react-loader";

const GUARD_SELECTOR_EDITOR_OPTIONS = {
  automaticLayout: true,
  fontFamily: "inherit",
  fontLigatures: false,
  fontSize: 14,
  fixedOverflowWidgets: true,
  glyphMargin: false,
  lineDecorationsWidth: 0,
  lineNumbers: "off",
  lineNumbersMinChars: 0,
  minimap: { enabled: false },
  overviewRulerBorder: false,
  overviewRulerLanes: 0,
  padding: { bottom: 4, top: 4 },
  renderFinalNewline: "off",
  renderLineHighlight: "none",
  roundedSelection: false,
  scrollbar: {
    alwaysConsumeMouseWheel: false,
    horizontal: "hidden",
    horizontalScrollbarSize: 0,
    useShadows: false,
    vertical: "hidden",
    verticalScrollbarSize: 0,
  },
  quickSuggestions: true,
  scrollBeyondLastLine: false,
  suggestOnTriggerCharacters: true,
  tabSize: 2,
  wordWrap: "off",
  wrappingIndent: "none",
} as const;

let monacoSetupState: "error" | "ready" = "ready";

if (import.meta.env.MODE !== "test") {
  try {
    registerWorkstationGuardSelectorMonaco(monaco);
    configureMonacoReactLoader(monaco);
  } catch {
    monacoSetupState = "error";
  }
}

interface MonacoGuardSelectorEditorProps {
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

export function MonacoGuardSelectorEditor({
  ariaDescribedBy,
  ariaInvalid = false,
  ariaLabel,
  className,
  hasError = false,
  height = "2.75rem",
  id,
  loadingMessage,
  modelPath,
  onChange,
  startupErrorMessage,
  value,
}: MonacoGuardSelectorEditorProps) {
  const onChangeRef = useRef(onChange);
  const startupState = monacoSetupState;
  const options = useMemo(() => GUARD_SELECTOR_EDITOR_OPTIONS, []);

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  if (startupState !== "ready") {
    return (
      <GuardSelectorEditorFallbackState
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
      language={WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID}
      onChange={(nextValue) => onChange(nextValue ?? "")}
      onMount={createGuardSelectorEditorMountHandler({ onChangeRef })}
      options={{ ...options, ariaLabel }}
      path={modelPath}
      theme={WORKSTATION_GUARD_SELECTOR_THEME_ID}
      value={value}
      width="100%"
      wrapperProps={{
        "aria-describedby": ariaDescribedBy,
        "aria-invalid": ariaInvalid ? "true" : undefined,
        "data-monaco-editor": "workstation-guard-selector",
        id,
      }}
    />
  );
}

function createGuardSelectorEditorMountHandler({
  onChangeRef,
}: {
  onChangeRef: RefObject<(value: string) => void>;
}) {
  return (
    editorInstance: MonacoEditorAPI.IStandaloneCodeEditor,
    monacoInstance: typeof import("monaco-editor"),
  ) => {
    const themeObserver =
      typeof MutationObserver === "undefined" || typeof document === "undefined"
        ? null
        : createGuardSelectorThemeObserver(
            monacoInstance,
            document.documentElement,
          );
    const completionProvider =
      registerWorkstationGuardSelectorCompletionProvider(monacoInstance);
    const contentChangeListener = editorInstance.onDidChangeModelContent(() => {
      onChangeRef.current(editorInstance.getValue());
    });
    const suggestListener = editorInstance.onDidChangeModelContent((event) => {
      const insertedText = event.changes.map((change) => change.text).join("");
      if (!insertedText.includes(".")) {
        return;
      }

      editorInstance.trigger(
        "workstation-guard-selector-autocomplete",
        "editor.action.triggerSuggest",
        {},
      );
    });

    editorInstance.onDidDispose(() => {
      themeObserver?.disconnect();
      completionProvider.dispose();
      contentChangeListener.dispose();
      suggestListener.dispose();
    });
  };
}

function createGuardSelectorThemeObserver(
  monaco: typeof import("monaco-editor"),
  root: HTMLElement,
) {
  const applyTheme = () => {
    applyWorkstationGuardSelectorTheme(monaco, root);
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

function GuardSelectorEditorFallbackState({
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
      data-monaco-editor-fallback="workstation-guard-selector"
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
