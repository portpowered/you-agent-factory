import { loader } from "@monaco-editor/react";

type MonacoModule = typeof import("monaco-editor");

let monacoReactLoaderConfigured = false;

export function configureMonacoReactLoader(monaco: MonacoModule) {
  if (monacoReactLoaderConfigured) {
    return;
  }

  loader.config({ monaco });
  monacoReactLoaderConfigured = true;
}

export function resetMonacoReactLoaderConfigurationForTests() {
  monacoReactLoaderConfigured = false;
}
