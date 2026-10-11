import { isAPIRecord } from "../transport";
import { messageQuery } from "./api";
import { parseAgentMessageObservation } from "./parsing";
import {
  type AgentMessageObservation,
  type MessageFilters,
  MessageReadError,
  type MessageReadOptions,
} from "./types";

/** One owned subscription. No EventSource reconnect, cookies, cursor or mark-read. */
export async function followAgentMessages(
  options: MessageReadOptions & MessageFilters,
  onObservation: (observation: AgentMessageObservation) => void,
): Promise<void> {
  const query = messageQuery({ ...options, status: undefined });
  query.set("follow", "true");
  let response: Response;
  try {
    response = await (options.fetch ?? globalThis.fetch)(
      `${options.origin}/messages?${query}`,
      {
        method: "GET",
        credentials: "omit",
        signal: options.signal,
        headers: { Accept: "text/event-stream" },
      },
    );
  } catch {
    throw new MessageReadError("unavailable");
  }
  if (!response.ok) {
    let body: unknown;
    try {
      body = await response.json();
    } catch {
      /* Only safe codes escape. */
    }
    throw new MessageReadError(
      isAPIRecord(body) && body.code === "MESSAGING_DISABLED"
        ? "disabled"
        : response.status === 403
          ? "denied"
          : response.status === 410
            ? "gap"
            : "unavailable",
    );
  }
  if (
    !response.body ||
    !response.headers.get("content-type")?.includes("text/event-stream")
  ) {
    await response.body?.cancel();
    throw new MessageReadError("invalid");
  }
  const reader = response.body.getReader();
  const abort = () => {
    void reader.cancel().catch(() => {});
  };
  options.signal?.addEventListener("abort", abort, { once: true });
  const decoder = new TextDecoder();
  let pending = "";
  let event = "message";
  let data: string[] = [];
  try {
    while (!options.signal?.aborted) {
      const chunk = await reader.read();
      if (chunk.done) throw new MessageReadError("unavailable");
      pending += decoder.decode(chunk.value, { stream: true });
      if (pending.length + data.join("\n").length > 1_048_576)
        throw new MessageReadError("invalid");
      let newline = pending.indexOf("\n");
      while (newline >= 0) {
        const line = pending.slice(0, newline).replace(/\r$/, "");
        pending = pending.slice(newline + 1);
        if (!line) {
          if (data.length) {
            let value: unknown;
            try {
              value = JSON.parse(data.join("\n"));
            } catch {
              throw new MessageReadError("invalid");
            }
            if (event === "error")
              throw new MessageReadError(
                isAPIRecord(value) && value.code === "MESSAGE_STREAM_GAP"
                  ? "gap"
                  : "unavailable",
              );
            if (event === "message")
              onObservation(parseAgentMessageObservation(value));
          }
          data = [];
          event = "message";
        } else if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:"))
          data.push(line.slice(5).replace(/^ /, ""));
        newline = pending.indexOf("\n");
      }
    }
  } catch (error) {
    if (!options.signal?.aborted)
      throw error instanceof MessageReadError
        ? error
        : new MessageReadError("unavailable");
  } finally {
    options.signal?.removeEventListener("abort", abort);
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
