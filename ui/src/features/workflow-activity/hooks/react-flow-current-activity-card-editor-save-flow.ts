import { useRef } from "react";

export function useGraphEditorLeaveEditorBridge() {
  const leaveEditorRef = useRef<() => void>(() => {});
  const setIsConfirmingLeaveEditorRef = useRef<(open: boolean) => void>(
    () => {},
  );

  return {
    bindSaveFlow(saveFlow: {
      leaveEditor: () => void;
      setIsConfirmingLeaveEditor: (open: boolean) => void;
    }) {
      setIsConfirmingLeaveEditorRef.current =
        saveFlow.setIsConfirmingLeaveEditor;
      leaveEditorRef.current = saveFlow.leaveEditor;
    },
    sessionCallbacks: {
      onAttemptLeaveEditor: () => setIsConfirmingLeaveEditorRef.current(true),
      onLeaveEditor: () => {
        leaveEditorRef.current();
      },
    },
  };
}
