import type { listAgentMessages } from "../../../api/agent-messages/api";
import type { followAgentMessages } from "../../../api/agent-messages/follow";
import {
  type AgentMessageObservation,
  type AgentMessagePage,
  type MessageListOptions,
  MessageReadError,
} from "../../../api/agent-messages/types";
import {
  matchesMessageFilters,
  reconcileMessagePage,
  retainMessageObservation,
} from "./message-page";

export interface MessageFeedSnapshot {
  page: AgentMessagePage;
  busy: boolean;
  live: boolean;
  newData: boolean;
  error: MessageReadError | null;
}

/** One bounded retained page and its owned source-native stream. */
export class MessageFeed {
  private controller = new AbortController();
  private streamController = new AbortController();
  private observations = new Map<string, AgentMessageObservation>();
  private page: AgentMessagePage = { messages: [] };
  private busy = false;
  private pendingPull = false;
  private firstFrame = true;
  private live = false;
  private newData = false;
  private error: MessageReadError | null = null;

  public constructor(
    private options: MessageListOptions,
    private readPage: typeof listAgentMessages,
    private readFollow: typeof followAgentMessages,
    private onState: (snapshot: MessageFeedSnapshot) => void,
    initialPage: AgentMessagePage = { messages: [] },
  ) {
    this.page = initialPage;
  }

  public start() {
    // An idle follow may never flush headers. Pull without waiting for readiness.
    const { nextToken: _cursor, ...filters } = this.options;
    void this.readFollow(
      { ...filters, status: undefined, signal: this.streamController.signal },
      this.observe,
    ).catch(this.fail);
    void this.pull();
  }

  public close() {
    this.controller.abort();
    this.streamController.abort();
  }

  private publish() {
    if (this.controller.signal.aborted) return;
    this.onState({
      page: {
        ...this.page,
        messages: reconcileMessagePage(
          this.page.messages,
          this.observations,
          this.options,
        ),
      },
      busy: this.busy,
      live: this.live,
      newData: this.newData,
      error: this.error,
    });
  }

  private fail = (cause: unknown) => {
    if (this.controller.signal.aborted) return;
    this.error =
      cause instanceof MessageReadError ? cause : new MessageReadError("error");
    this.live = false;
    this.busy = false;
    this.streamController.abort();
    this.publish();
    this.controller.abort();
  };

  public pull = async () => {
    if (this.controller.signal.aborted) return;
    if (this.busy) {
      this.pendingPull = true;
      return;
    }
    this.busy = true;
    this.publish();
    try {
      const result = await this.readPage({
        ...this.options,
        signal: this.controller.signal,
      });
      if (this.controller.signal.aborted) return;
      this.page = result;
      for (const id of this.observations.keys()) {
        if (!this.page.messages.some((message) => message.messageId === id))
          this.observations.delete(id);
      }
      this.busy = false;
      this.publish();
      if (this.pendingPull) {
        this.pendingPull = false;
        void this.pull();
      }
    } catch (cause) {
      this.fail(cause);
    }
  };

  private observe = (observation: AgentMessageObservation) => {
    if (this.controller.signal.aborted) return;
    const message = observation.message;
    if (!matchesMessageFilters(message, { ...this.options, status: undefined }))
      return;
    this.live = true;
    const loaded = this.page.messages.some(
      (row) => row.messageId === message.messageId,
    );
    if (loaded || this.busy) {
      if (
        this.observations.size >= 200 &&
        !this.observations.has(message.messageId)
      ) {
        this.fail(new MessageReadError("gap"));
        return;
      }
      if (!retainMessageObservation(this.observations, observation)) return;
    }
    if (!loaded && matchesMessageFilters(message, this.options))
      this.newData = true;
    this.publish();
    if (this.firstFrame) {
      this.firstFrame = false;
      void this.pull();
    }
  };
}
