import { useEffect, useRef, useState } from "react";
import { listAgentMessages } from "../../../api/agent-messages/api";
import { followAgentMessages } from "../../../api/agent-messages/follow";
import type {
  MessageFilters,
  MessageReadError,
} from "../../../api/agent-messages/types";
import { factoryAPIURL } from "../../../api/baseUrl";
import { MessageFeed, type MessageFeedSnapshot } from "../lib/message-feed";

export interface MessageFeedOptions {
  origin?: string;
  enabled?: boolean;
  filters?: MessageFilters;
  list?: typeof listAgentMessages;
  follow?: typeof followAgentMessages;
  scopeKey?: string;
}

const emptyPage: MessageFeedSnapshot["page"] = { messages: [] };

/** Disposable host/filter/page generation; no Factory projection or message mutation. */
export function useMessageFeed({
  origin = factoryAPIURL("").replace(/\/$/, ""),
  enabled = true,
  filters = {},
  list = listAgentMessages,
  follow = followAgentMessages,
  scopeKey,
}: MessageFeedOptions) {
  const filterKey = JSON.stringify(filters);
  const readers = useRef({ list, follow });
  readers.current = { list, follow };
  const key = JSON.stringify([origin, filterKey, scopeKey]);
  const [walk, setWalk] = useState({
    key,
    cursors: [undefined] as (string | undefined)[],
    revision: 0,
  });
  const current =
    walk.key === key ? walk : { key, cursors: [undefined], revision: 0 };
  const cursor = current.cursors.at(-1);
  const generation = JSON.stringify([key, cursor, current.revision, enabled]);
  const [state, setState] = useState({
    generation: "",
    key: "",
    page: emptyPage,
    busy: true,
    live: false,
    newData: false,
    error: null as MessageReadError | null,
  });
  const lastState = useRef(state);
  lastState.current = state;
  const visible =
    state.generation === generation
      ? state
      : {
          ...state,
          page: state.key === key ? state.page : emptyPage,
          busy: enabled,
          live: false,
          newData: false,
          error: null,
        };

  useEffect(() => {
    if (!enabled) return;
    const feed = new MessageFeed(
      { origin, ...JSON.parse(filterKey), nextToken: cursor },
      readers.current.list,
      readers.current.follow,
      (snapshot) => setState({ generation, key, ...snapshot }),
      lastState.current.key === key ? lastState.current.page : emptyPage,
    );
    feed.start();
    const refreshVisible = () => {
      if (document.visibilityState !== "hidden") void feed.pull();
    };
    const timer = globalThis.setInterval(refreshVisible, 30_000);
    const online = () => {
      // A fresh generation replaces the process-local follow stream and cursor walk.
      setWalk((previous) => ({
        key,
        cursors: [undefined],
        revision: previous.revision + 1,
      }));
    };
    document.addEventListener("visibilitychange", refreshVisible);
    window.addEventListener("online", online);
    return () => {
      feed.close();
      globalThis.clearInterval(timer);
      document.removeEventListener("visibilitychange", refreshVisible);
      window.removeEventListener("online", online);
    };
  }, [origin, filterKey, cursor, generation, enabled, key]);

  return {
    messages: visible.page.messages,
    busy: visible.busy,
    error: visible.error,
    live: visible.live,
    newData: visible.newData,
    unavailable: !enabled,
    previousAvailable: current.cursors.length > 1,
    nextAvailable: Boolean(visible.page.nextToken),
    refresh: () =>
      setWalk({ key, cursors: [undefined], revision: current.revision + 1 }),
    previous: () =>
      setWalk({ ...current, cursors: current.cursors.slice(0, -1) }),
    next: () => {
      if (visible.page.nextToken)
        setWalk({
          ...current,
          cursors: [...current.cursors, visible.page.nextToken],
        });
    },
  };
}

export type MessageFeedState = ReturnType<typeof useMessageFeed>;
