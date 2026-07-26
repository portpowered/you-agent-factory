export const editor = {
  defineTheme() {},
  setModelMarkers() {},
  setTheme() {},
};

export const languages = {
  CompletionItemKind: { Variable: 4 },
  CompletionTriggerKind: { Invoke: 0, TriggerCharacter: 1 },
  register() {},
  registerCompletionItemProvider: () => ({
    dispose() {},
  }),
  setMonarchTokensProvider() {},
};
