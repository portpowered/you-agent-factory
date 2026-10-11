# Agent Messages

An Agent Message is a durable message between permitted Worker Sessions.
Messages have their own store and inbox, separate from Factory Events and Provider Session transcripts.
Messaging never creates Work, acquires execution slots, or changes Factory execution.

## Caller authority

Sending requires the execution credentials of an active `RUNNING` Worker Session.
The CLI reads `YOU_WORKER_SESSION_ID` and `YOU_WORKER_SESSION_TOKEN` from the running worker's environment.
The server validates both against its current owner.
Credentials become invalid after termination, owner loss, or process restart.
Keep credentials private and use the execution-provided values.

A worker may address its evidenced lead or requester chain.
A lead may address its own Project workers.
Matching project labels alone grant no permission.
Sibling workers, unrelated projects, and roots without requester relationships have no default send permission.
Operators may observe messages but cannot send or reply.

## Send and reply

From an authorized running worker, send a message to its permitted recipient:

```sh
you message recipient-worker-session-id "Please review the result" --request-id review-one --output json
```

The response contains a stable `messageId` and its durable delivery status.
The server persists the message before returning HTTP `202`.
Specify `--server http://127.0.0.1:7437` to select a host explicitly.
Otherwise, the CLI uses `YOU_SERVER` when available.

Use `--worker-session-id` to supply the recipient instead of a positional address.
Use `--session` to disambiguate a recipient's legacy Worker Session ID by Factory Session.
The scope selector grants no authority.
An ambiguous ID returns `WORKER_SESSION_AMBIGUOUS` with candidates.

Use `--body` for an explicit body, positional words for ordinary text, or piped stdin when neither is supplied.
An empty body fails validation.
Use `--body-secret` to declare the entire body secret.
The server redacts declared content before persistence, publication, and hashing.
Responses report `bodyRedactionCount` and the hash of the redacted body.
Caller credentials appearing in the body also trigger conservative redaction.

From the recipient's running worker, reply to the original message:

```sh
you message reply msg-original "Review complete" --request-id review-reply --output json
```

The default reply recipient is the original sender Worker Session.
The reply preserves `threadId` and sets `inReplyTo`.
The original becomes `REPLIED` with `repliedByMessageId` in the same durable transaction.
An explicit reply recipient still requires send permission.

Retry a disconnected send with the same `--request-id` and privacy-normalized request.
The server returns the original record without spending another successful-send allowance.
A changed request under the same sender/request key returns `MESSAGE_REQUEST_CONFLICT`.
Identical body/recipient sends within ten minutes also return the original message.

## Read the inbox

From a permitted recipient, pull its inbox:

```sh
you message list --to-me --output json
```

`--to-me` includes the verified continuation chain and exact own-Work identity.
It does not include messages merely because project labels match.
Without worker credentials, list is read-only operator observation.
Partial or invalid worker credentials fail validation instead of becoming operator access.

Mark permitted inbox messages read:

```sh
you message list --to-me --mark-read --output json
```

Only a permitted recipient can change `QUEUED` to `READ`.
Sender and operator observation leave another recipient's status unchanged.
`REPLIED` and `EXPIRED` never regress to `READ`.

Filters combine with AND after authorization:

| CLI flag | HTTP query field |
| --- | --- |
| `--to-me` | `toMe` |
| `--to-worker-session-id` | `toWorkerSessionId` |
| `--from-worker-session-id` | `fromWorkerSessionId` |
| `--thread` | `threadId` |
| `--correlation-work-id` | `correlationWorkId` |
| `--correlation-factory-session` | `correlationFactorySessionId` |
| `--status` (repeatable) | `status` (repeatable) |
| `--max-results` | `maxResults` |
| `--next-token` | `nextToken` |
| `--mark-read` | `markRead` |
| `--follow` | `follow` |

Pages follow original send order.
The default page contains at most 100 messages, with a maximum requested size of 500.
Reuse `nextToken` with the same caller and filters.
The cursor cannot grant read authority or expose messages admitted after its page snapshot.

For live observations, follow the permitted inbox:

```sh
you message list --to-me --follow --output json
```

Follow emits source-native message observations from the process-local Events stream.
Each delivered observation revalidates authorization.
Follow does not mark messages read and does not survive process reconstruction.
After disconnect, retention gaps, or backpressure, pull the durable inbox before starting another follow.

## HTTP contract

CLI and HTTP share authorization, validation, durable transitions, and structured errors.
Worker requests supply these execution-only headers:

```text
Authorization: Bearer <execution-provided-token>
X-You-Worker-Session-Id: <execution-provided-worker-session-id>
```

| Operation | Request | Result |
| --- | --- | --- |
| Send | `POST /messages` | `202` with `{"message": ...}` after durable commit |
| Reply | `POST /messages` with `inReplyTo` | Same send result and atomic original status update |
| Inbox | `GET /messages?toMe=true` | Authorized `messages` page and optional `nextToken` |
| Mark read | `GET /messages?toMe=true&markRead=true` | Recipient-only durable status changes |
| Get | `GET /messages/{message_id}` | Authorized message, marking it read for its recipient |
| Follow | `GET /messages?toMe=true&follow=true` | `text/event-stream` observations |

A send body uses the scope-free recipient address:

```json
{
  "requestId": "review-one",
  "to": {"workerSessionId": "recipient-worker-session-id"},
  "body": "Please review the result"
}
```

A reply may omit `to` and supply `inReplyTo` instead.
Use the separate `factorySessionId` query parameter for legacy recipient disambiguation.
Correlation fields describe related Work or Factory Sessions and grant no authority.

## Limits and durable recovery

The selected operator configuration accepts these defaults:

```json
{
  "messaging": {
    "enabled": true,
    "retentionDays": 30,
    "policy": {
      "scopeLabelKeys": ["tag:project"],
      "allowCrossScope": false
    },
    "limits": {
      "sendsPerChainPerHour": 10,
      "revivesPerChainPerHour": 6,
      "messagesPerThread": 8,
      "maxBodyBytes": 8192,
      "maxHop": 3,
      "defaultExpirySeconds": 86400
    }
  }
}
```

Successful sends share a rolling one-hour allowance across the sender's continuation chain.
Denied and failed writes do not spend that allowance.
Body limits count UTF-8 bytes before redaction.
Use `--expires 1h`, or HTTP `expiresInSeconds`, to select an expiry between one minute and seven days.
The default expiry is 24 hours.
Read and admission operations durably expire due messages.

Retention removes complete settled threads older than the configured window during startup.
Pending messages and retained thread relationships remain available.
Reconstructing the process with the same isolated profile recovers identities, redacted content, statuses, reply relationships, and request deduplication.
Startup also reconciles interrupted continuations through the original request, input, and successor identity.
Confirmed provider progress produces durable delivery facts without another provider attempt.
Recorded provider-classified rejection retains `REVIVE_UNSUPPORTED` and the failed successor after restart.
That rejected attempt does not consume successful revival volume.
Unavailable or incomplete evidence keeps the message queued with `REVIVE_FAILED`.
Separate processes must not share a writable messaging profile.

Setting `messaging.enabled` to `false` returns `MESSAGING_DISABLED` with HTTP `503` and preserves stored messages.
Invalid messaging configuration or store faults affect Messaging without failing Factory execution.
Corrupt storage returns `MESSAGE_STORE_CORRUPT` and preserves existing bytes.
Unavailable storage returns `MESSAGE_STORE_UNAVAILABLE`.
Repair the underlying storage before reconstructing the process.
Do not include message bodies or execution credentials in diagnostic reports.

## Delivery to ended recipients

Running recipients receive queued inbox records without execution changes.
The default `ifEnded` is `REVIVE`.
An ended eligible recipient resumes its recorded Provider Session in one direct successor, using the normalized message body.
Messaging does not create Work or acquire a Factory execution slot.
Admission leaves the message `QUEUED` with `REVIVE_PENDING` while the provider resumes.
Send returns without waiting for provider progress.
Provider-confirmed resumed progress and durable delivery produce `DELIVERED_REVIVE` with `revivedWorkerSessionId` and `to.deliveredWorkerSessionId`.
Use message read or follow to observe that outcome.
Concurrent sends never branch the recipient chain.
Messages sent while its newest successor is running remain queued.
When that recipient ends, Messaging resumes its recorded Provider Session once with eligible unread `REVIVE` bodies in original send order.
The original messages retain their IDs and share the confirmed successor.
`HOLD`, `READ`, `REPLIED`, and `EXPIRED` messages do not trigger this delivery.

Missing continuation capability leaves `QUEUED` with `REVIVE_UNSUPPORTED`.
Provider-classified resume rejection also leaves the message queued with `REVIVE_UNSUPPORTED`; the successor ends `FAILED`.
Rejected or uncertain admission leaves `QUEUED` with `REVIVE_FAILED`.
The default rolling limit permits six provider-confirmed revivals per recipient chain per hour.
Set `messaging.limits.revivesPerChainPerHour` in the selected profile to change this positive limit.
Further messages remain queued with `REVIVE_LIMIT`.
Failed attempts do not spend this limit.
Recovery preserves the original admission time for this rolling limit.
If an interrupted delivery has no saved admission time, Messaging reads the exact successor's recorded opening time.
Missing or inconsistent timing evidence leaves the message queued with `REVIVE_FAILED`.
Retries preserve the original message and never spend another revival.
Use `--if-ended hold` to request queued retention explicitly.
Use `--reply-hold` to make a future reply inherit `HOLD`.

`--interrupt` returns `MESSAGE_INTERRUPT_UNSUPPORTED` before any effect.
Unreplied messages become `EXPIRED` at `expiresAt`, even without a read request.
Messaging schedules expiry independently of Factory execution.
Replies preserve the parent message as `REPLIED`.
Interruption, MCP messaging, dashboard messaging, and prompt adoption belong to later increments.
No Factory route, guard, or workstation waits on an Agent Message.

For Worker Session discovery and execution identity, run `you docs operations`.
For canonical Factory recording and replay, run `you docs record-replay`.
