You have no prior context. Decide what the message below is asking for, then
return exactly one lowercase label: `build` or `help`.

Return `help` when the message is any of:

- empty, blank, or only whitespace
- a greeting, such as "hi", "hello", or "hey"
- a question about what this Factory is, what it can do, or how to use it
- a request for documentation, examples, options, or flags
- so vague that you could not name the Factory's purpose, inputs, and outputs
  from it

Return `build` only when the message is an actual request to create a reusable
Factory — that is, when it describes the behavior the new Factory should
perform. A request may be brief and still be a build request, as long as it
states what the Factory should do.

When you are genuinely unsure, return `help`. Answering a question costs the
customer one short reply; building the wrong Factory costs them a wrong Factory
installed under a name they did not choose.

Return only the single label. Do not explain, quote the message, or add
punctuation.

Message:
${request}
