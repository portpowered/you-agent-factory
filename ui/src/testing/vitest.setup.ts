import "./guarded-suite-console.setup";

import { configure } from "@testing-library/react";
import { installResizeObserverShim } from "../components/dashboard/test-browser-shims";
import { monacoEditorReactMock } from "./vitest-monaco-editor-mock";

installResizeObserverShim();

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

vi.mock("@monaco-editor/react", () => monacoEditorReactMock);

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
