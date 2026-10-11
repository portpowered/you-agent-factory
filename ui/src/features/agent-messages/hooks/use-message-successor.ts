import { useEffect, useRef, useState } from "react";
import type {
  AgentMessage,
  MessageWorkerIdentity,
} from "../../../api/agent-messages/types";
import {
  type MessageIdentityReader,
  readConfirmedMessageSuccessor,
} from "../lib/read-message-scope";

export function useMessageSuccessor(
  origin: string,
  scopeKey: string,
  onNavigate?: (identity: MessageWorkerIdentity) => void,
  readIdentity?: MessageIdentityReader,
) {
  const [error, setError] = useState(false);
  const navigation = useRef<AbortController | null>(null);
  const activeScope = useRef(scopeKey);
  useEffect(() => {
    activeScope.current = scopeKey;
    setError(false);
    return () => navigation.current?.abort();
  }, [scopeKey]);
  return {
    error,
    open: onNavigate
      ? async (message: AgentMessage) => {
          navigation.current?.abort();
          const controller = new AbortController();
          navigation.current = controller;
          setError(false);
          try {
            const identity = await readConfirmedMessageSuccessor(
              message,
              { origin, signal: controller.signal },
              readIdentity,
            );
            if (!controller.signal.aborted) onNavigate(identity);
          } catch {
            if (!controller.signal.aborted) setError(true);
          }
        }
      : undefined,
  };
}
