import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import {
  type CurrentActivityGraphViewModelInput,
  useCurrentActivityGraphViewModel,
} from "./react-flow-current-activity-card-graph-view-model";

export function useCurrentActivityGraphCardViewModel(
  input: CurrentActivityGraphViewModelInput,
) {
  const graph = useCurrentActivityGraphViewModel(input);

  return {
    ...input.editor,
    ...graph,
  };
}

export type CurrentActivityGraphCardViewModel = ReturnType<
  typeof useCurrentActivityGraphCardViewModel
>;

export type CurrentActivityGraphEditorViewModel = ReturnType<
  typeof useCurrentActivityGraphEditor
>;
