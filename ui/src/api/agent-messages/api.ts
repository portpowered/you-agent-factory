import { isAPIRecord } from "../transport";
import { parseAgentMessagePage, parseMessageWorkerIdentity } from "./parsing";
import {
  type MessageListOptions,
  MessageReadError,
  type MessageReadOptions,
} from "./types";

async function read(
  options: MessageReadOptions,
  path: string,
): Promise<unknown> {
  let response: Response;
  try {
    response = await (options.fetch ?? globalThis.fetch)(
      `${options.origin}${path}`,
      {
        method: "GET",
        signal: options.signal,
        credentials: "omit",
      },
    );
  } catch {
    throw new MessageReadError("unavailable");
  }
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new MessageReadError("invalid");
  }
  if (response.ok) return body;
  const disabled = isAPIRecord(body) && body.code === "MESSAGING_DISABLED";
  throw new MessageReadError(
    disabled
      ? "disabled"
      : response.status === 403
        ? "denied"
        : response.status === 409
          ? path.startsWith("/messages?")
            ? "expired-page"
            : "unavailable"
          : response.status === 404 || response.status === 503
            ? "unavailable"
            : "error",
  );
}

export async function listAgentMessages(options: MessageListOptions) {
  const query = messageQuery(options);
  if (options.nextToken) query.set("nextToken", options.nextToken);
  return parseAgentMessagePage(await read(options, `/messages?${query}`));
}

export function messageQuery(options: import("./types").MessageFilters) {
  const query = new URLSearchParams({ maxResults: "100" });
  for (const key of [
    "factorySessionId",
    "toWorkerSessionId",
    "fromWorkerSessionId",
    "threadId",
  ] as const) {
    if (options[key]) query.set(key, options[key]);
  }
  for (const status of options.status ?? []) query.append("status", status);
  return query;
}

export async function readMessageWorkerIdentity(
  options: MessageReadOptions & {
    workerSessionId: string;
    factorySessionId?: string;
  },
) {
  const query = new URLSearchParams();
  if (options.factorySessionId)
    query.set("factorySessionId", options.factorySessionId);
  const suffix = query.size ? `?${query}` : "";
  return parseMessageWorkerIdentity(
    await read(
      options,
      `/worker-sessions/${encodeURIComponent(options.workerSessionId)}${suffix}`,
    ),
  );
}
