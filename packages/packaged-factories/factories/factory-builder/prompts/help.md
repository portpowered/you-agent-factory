The customer has not asked you to build anything yet. Do not create, install,
validate, or modify any Factory. Do not read the workspace or run any command.

Reply with the guidance block below reproduced verbatim. You may add at most
one short sentence before it acknowledging what the customer said. Add nothing
after it, and never invent flags, commands, capabilities, or documentation
topics that do not appear in it.

---

**Factory Builder** creates one reusable Factory from a description of what you
want it to do, then installs it so you can run it by name.

Tell me what the Factory should do, and I will build it. For example:

- "Create a Factory that reviews release notes and returns an approved summary."
- "Build a Factory that runs two independent analyses and merges them into one answer."

**Two forms are available**

- `graph` — a YAML topology of workstations and work states. This is the default.
- `javascript` — a JavaScript orchestrator, for logic that is awkward as a graph.

**From the command line**

```
you run --named @you/factory-builder --to "<what the Factory should do>"
```

Optional flags:

- `--factory-name <name>` — a stable name to install under. Derived from your request when omitted.
- `--orchestrator graph|javascript` — which form to author. Defaults to `graph`.

**From an editor over ACP**

Just describe the Factory you want in a message. No flags are needed.

**Documentation**

- `you docs authoring-factories` — work types, states, workstations, and guards
- `you docs javascript-workflows` — the JavaScript orchestrator form
- `you docs packaged-factories` — the Factories that ship with `you`
- `you docs run` — invoking a Factory and passing inputs

---

Customer message:
${request}
