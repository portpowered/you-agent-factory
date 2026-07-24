import { useMemo, useState } from "react";

import "../../../../../styles.css";
import { ReactFlowCurrentActivityCard } from "../../../../workflow-activity/components/react-flow-current-activity-card";
import type { CurrentActivitySelection } from "../../../../workflow-activity/lib/react-flow-current-activity-card-types";
import {
  buildVisualGroupEditorFetchMocks,
  buildVisualGroupEditorSnapshot,
} from "./factory-graph-visual-group-editor-story-state";

function VisualGroupEditorStory() {
  const [selection, setSelection] = useState<CurrentActivitySelection | null>(
    null,
  );
  const snapshot = useMemo(() => buildVisualGroupEditorSnapshot(), []);

  return (
    <div data-visual-group-editor-story="" style={{ minHeight: "760px" }}>
      <ReactFlowCurrentActivityCard
        now={Date.parse("2026-06-14T12:00:00Z")}
        onSelectDoc={() => {}}
        onSelectResource={() => {}}
        onSelectStateNode={(placeId) =>
          setSelection({ kind: "state-node", placeId })
        }
        onSelectWorkID={() => {}}
        onSelectWorker={() => {}}
        onSelectWorkType={() => {}}
        onSelectWorkstation={(nodeId) => setSelection({ kind: "node", nodeId })}
        selection={selection}
        snapshot={snapshot}
      />
    </div>
  );
}

export default {
  title: "Factory Graph Editor/Visual Groups",
  tags: ["test"],
};

export const EditorSaveReloadWorkflow = {
  parameters: {
    dashboardApi: {
      fetchMocks: buildVisualGroupEditorFetchMocks(),
      snapshot: buildVisualGroupEditorSnapshot(),
    },
  },
  render: () => <VisualGroupEditorStory />,
};
