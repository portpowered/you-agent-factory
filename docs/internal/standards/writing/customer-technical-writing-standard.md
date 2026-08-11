# Customer Technical-Writing Standard

Status: **Normative**
Owner: Documentation maintainers, with product meaning owned by the relevant
architecture or public-contract owner
Applies to: Customer-facing labels, procedures, descriptions, examples, and
technical explanations

This standard is the repository contract for customer-facing technical prose.
Use it when you author or review CLI help, API descriptions, reference pages,
architecture explanations intended for customers, error messages, examples,
and other text that tells a customer what the product means or does.

The standard uses **MUST**, **MUST NOT**, **SHOULD**, and **MAY** in their
ordinary normative sense. A literal product or protocol value remains exact
even when it does not follow a prose rule.

## Quick rules

1. Classify the text before editing it.
2. Preserve protected machine and technical text byte-for-byte unless the
   owning contract changes.
3. Use the canonical customer term for the intended meaning.
4. Write procedures as short, ordered actions with observable results.
5. Write descriptions in small, single-topic units.
6. Record a specific, owned exception when a rule cannot apply.
7. Ask a human reviewer to confirm meaning, safety, and contract fidelity.

This is a repository writing profile aligned with ASD-STE100. It is not a
redistribution of the ASD-STE100 dictionary, a certification, or a claim of
full ASD-STE100 conformance. See [Conformance and external
references](#conformance-and-external-references).

## Scope and authority

This standard governs the form of customer-facing prose. It does not redefine
product behavior. When wording conflicts with a public contract or the
[architecture data model](../../../architecture/data-model.md), preserve the
contract and ask its owner to correct the source definition or approve an
explicit exception.

The standard applies to:

- labels and short action or status text;
- procedures and task instructions;
- descriptions and explanatory paragraphs;
- commands, code examples, schemas, and other technical text;
- product terms and their approved forms; and
- warnings, failure guidance, and justified exceptions.

It does not require a rewrite of existing documentation. Apply it when
creating or materially revising customer prose. This story establishes the
policy only; it does not implement a prose checker, a baseline, a CI gate, or
an automated corpus migration.

## Content classes

Classify each sentence or block by its primary purpose. Use the most
restrictive applicable rule when a block contains more than one class.

| Class | Use it for | Required shape |
| --- | --- | --- |
| **Label** | A heading, button, menu item, CLI help name, status, or short field label. | Use a concise noun phrase for a thing or status. Use an imperative verb for an action. Use sentence case unless a protected name or literal requires another case. |
| **Procedural prose** | A step that tells a customer what to do. | Use one action per sentence, an imperative verb, required conditions before the action, and an observable result after the action. Use an ordered or vertical list when order matters. |
| **Descriptive prose** | An explanation of what a resource, option, state, or behavior means. | Introduce information gradually. Keep one topic per paragraph. Prefer active voice and stable key words for related ideas. |
| **Machine and technical text** | A command, API method or route, path, schema, identifier, status, error code, configuration value, or external output. | Preserve the literal exactly. Explain it around the literal instead of rewriting it. |
| **Technical term** | A product noun, action, relationship, lifecycle state, or implementation term with a specific meaning. | Use the canonical spelling and capitalization. Define a customer term before relying on it in a new context. Keep internal terms in explicitly internal material. |

### Labels

Labels are scanned rather than read as sentences. They MUST:

- use the registered customer term when the label names a product concept;
- distinguish an action from a state (for example, `Create Factory` is an
  action and `Factory Session` is a resource name);
- use sentence case for ordinary labels, while preserving protected names,
  acronyms, and command literals;
- avoid filler such as “click here”, “option for”, or “manage”; and
- remain short enough for the surface without dropping the meaning or hiding a
  destructive effect.

An action label should identify the result, such as `Create Factory` or
`Cancel Factory Session`. A status label should identify the state, such as
`Paused` or `Completed`. Do not turn a product noun into an unregistered
synonym merely to make a label shorter.

### Procedural prose

Procedures MUST follow all of these rules:

- Use no more than **20 words per sentence**.
- Put necessary conditions before the instruction that depends on them.
- Use one instruction per sentence unless the actions must occur
  simultaneously.
- Begin the instruction with an imperative verb when the sentence tells the
  reader to act.
- State an observable expected result after the action or in the next step.
- Use an ordered or vertical list when the order of actions matters.
- Keep warnings, destructive effects, and recovery actions explicit.

Count words in the natural-language portion of the sentence. Protected code,
commands, identifiers, and other literals are not rewritten to meet the word
limit. Do not use a long literal as a reason to hide additional prose in the
same sentence.

When two actions must happen together, say so and explain the dependency. Do
not combine unrelated actions to reduce the apparent step count.

### Descriptive prose

Descriptions MUST follow all of these rules:

- Use no more than **25 words per sentence**.
- Introduce the subject before its details, and add details gradually.
- Keep one topic per paragraph.
- Use no more than **six sentences per paragraph**.
- Prefer active voice when it preserves the meaning.
- Repeat stable key words or phrases for related information instead of
  alternating between synonyms.
- State the customer-visible consequence before implementation detail.

These limits support scanning and translation. They do not permit a writer to
remove a condition, qualification, warning, or recovery path that a customer
needs.

### Machine and technical text

Treat machine and technical text as a contract boundary. Put explanation
outside the protected span. Do not replace punctuation, capitalization,
spelling, separators, quoting, or whitespace inside a literal unless the
owning contract or source artifact changes.

For a customer example, identify the literal and then explain the expected
behavior. If the example is copied from a shipped contract, verify it against
the source before publication.

### Technical terms

Product terms name customer concepts. Use the public vocabulary in the
[architecture data model](../../../architecture/data-model.md), public API
contracts, and the terminology register maintained with this standard. Use
the same capitalization for the same meaning. Use an approved plural or verb
form when the term record provides one.

Define a term on first use when a reader could confuse it with an ordinary
English word or with another product concept. Do not introduce an internal
implementation term as if it were a customer resource. Terms such as `CPN`,
`place`, `transition`, `marking`, and `token` belong only in material marked
internal or implementation-focused unless a public contract explicitly
exposes one of them.

## Protected machine and technical text

The following text is protected. A prose review MUST distinguish the literal
from the natural-language explanation around it:

- fenced code blocks and inline code spans;
- commands, arguments, flags, quoting, redirects, pipes, `&&`, `||`, `|`, `;`,
  glob patterns, and other shell operators;
- JSON and YAML contract examples, including keys, values, indentation that
  affects meaning, and required quoting;
- API methods and routes, such as `POST /factory-sessions/sync`;
- filesystem paths, URLs, URI schemes, package paths, environment variables,
  and configuration keys;
- identifiers, field names, schema names, event names, status values, and
  error-code literals;
- model names, provider names, command names, and other registered external
  names;
- quoted external output, including output copied to document a failure; and
- generated artifacts and generated excerpts whose source owns the exact
  representation.

Protected does not mean “skip review.” Reviewers MUST check that the literal
comes from the correct shipped source, appears in the correct context, and is
explained accurately. A natural-language comment inside a code block remains
reviewable when it is safely identifiable as prose. For example, review the
comment in this snippet, but preserve the command and flag literals:

```sh
# Start the local server before sending the request.
you server --listen 127.0.0.1:7437
```

If a protected value is wrong, update the owning source or report the contract
defect. Do not “fix” it by changing only a documentation copy.

## Rule sets and review boundary

The governing plan separates deterministic findings, advisory findings, and
human language review. This standard preserves that boundary:

### Blocking deterministic rules

These findings are blocking when the text is in the applicable class:

- **B-PROC-20:** a procedural sentence exceeds 20 natural-language words;
- **B-DESC-25:** a descriptive sentence exceeds 25 natural-language words;
- **B-PARA-6:** a descriptive paragraph exceeds six sentences;
- **B-LITERAL:** a protected literal differs from its owning contract or
  source artifact; and
- **B-TERM:** a customer concept uses a known canonical term with different
  spelling or capitalization, without an approved exception.

### Advisory deterministic rules

These findings help a reviewer find likely problems but do not prove that the
prose is wrong:

- **A-PASSIVE:** flag passive constructions for review;
- **A-ORDER:** flag procedural paragraphs that use a sequence but do not use a
  vertical or ordered list;
- **A-KEYWORD:** flag related descriptions that alternate between near-
  synonyms; and
- **A-FORM:** flag a possible unapproved plural or verb form for a registered
  term.

### Human-review rules

Human review is required for judgments that depend on meaning or context:

- **H-ACTION:** confirm that each instruction has the right imperative verb,
  enough conditions, and one understandable action;
- **H-RESULT:** confirm that the expected result is observable and matches the
  actual behavior;
- **H-SEQUENCE:** confirm the order and dependency of steps;
- **H-MEANING:** confirm product, technical, and subject-matter accuracy;
- **H-VOICE:** accept passive voice only when the actor is unknown,
  irrelevant, or intentionally omitted;
- **H-CONTRACT:** confirm examples, warnings, destructive operations, and
  recovery guidance preserve the shipped contract; and
- **H-EXCEPTION:** confirm that an exception is necessary, bounded, owned, and
  reviewable.

These labels define policy and review severity. They do not mean that a prose
checker already exists or that a future checker can decide semantic
correctness by itself.

## Exceptions and suppressions

An exception is a documented reason that one rule cannot apply to one exact
span. It is not permission to lower the standard for a whole document.

An exception record MUST name:

1. the rule ID, such as `B-PROC-20`;
2. the exact sentence, literal, or block in scope;
3. the reason the rule would change the required meaning;
4. the owning reviewer or product owner; and
5. an expiry date or review point when the exception depends on a temporary
   compatibility, legal, safety, or external-contract condition.

Valid reasons include:

- a protected literal or quoted external output must remain exact;
- two actions must occur simultaneously and separating them would imply a
  false order;
- a legal, safety, security, or compatibility warning must preserve approved
  wording;
- generated output cannot be shortened without changing its source contract;
  or
- a public technical name is owned by an external contract and has no safe
  local synonym.

Do not suppress a finding because the author prefers a longer sentence, did
not have time to revise it, or considers ordinary readability optional. Do not
suppress `B-LITERAL` to conceal a documentation and contract mismatch. Fix the
source or obtain an owner decision.

For a manual review, record an exception in this form:

```text
Exception: B-PROC-20
Scope: “Stop the server and remove its temporary directory at the same time.”
Reason: The cleanup operation must be simultaneous with shutdown for the
        documented recovery procedure.
Owner: Runtime maintainer
Review: Recheck when shutdown cleanup becomes independently observable.
```

A future checker MAY provide a machine-readable suppression syntax, but this
story does not implement or promise one. Any such syntax MUST retain the same
rule, scope, reason, owner, and review-boundary requirements.

## Repository examples

The examples below use shipped Factory vocabulary and contract-shaped text.
The bad examples are intentionally close to common authoring mistakes.

### Command description

**Good:** `Lists the factories available to the current server.`

**Bad:** `This command is used for the purpose of displaying a list of all the
factories that are currently available.`

The good description states one observable behavior. The bad description adds
filler and hides the result in an indirect phrase.

### Procedure

**Good:**

1. Start the server at `127.0.0.1:7437`.
2. Run `you factory list`.
3. Confirm that the command prints the available factories.

**Bad:** `In order to see all the factories that are available, you should
start the server if it is not already running and then run the command, which
will hopefully print the list.`

The good procedure puts the condition and actions in order, uses imperative
verbs, and states an observable result. The bad procedure combines actions,
uses a weak verb, and does not give reliable recovery guidance.

### Explanation

**Good:** A `Factory` is a saved definition. A `Factory Session` runs one live
instance of that definition.

**Bad:** A Factory is basically the same thing as a Factory Session, and it
contains all of the live work, workers, events, and runtime state that the
session may or may not execute.

The good explanation introduces one distinction at a time. The bad paragraph
collapses two resources and several unrelated topics into one sentence.

### Product term

**Good:** Submit a `Work Request` to create or update `Work`.

**Bad:** Submit a job to create a task.

The good example uses the public terms. The bad example replaces two defined
terms with informal synonyms and changes the product meaning.

### Protected technical example

**Good:** Send the request to the exact route below.

```http
POST /factory-sessions/sync
```

**Bad:** Send the request to `POST /factory-session/{id}/sync`.

The bad example changes both the route and its resource shape. A writer must
verify the route against the public contract rather than normalize it as
ordinary prose.

### Valid exception

**Good:** Keep `FACTORY_SESSION_OPENED` unchanged because it is a protected
event literal emitted by the recording contract. Record the `B-LITERAL`
exception with its contract owner.

**Bad:** Ignore the sentence limit because the author prefers this wording.

The first example identifies a protected literal, a rule, and an owner. The
second example is ordinary readability debt and is not a valid exception.

## Manual review checklist

Use this checklist for new or materially revised customer prose. Mark an item
not applicable only when the reviewer records why.

### Classification and vocabulary

- [ ] Each block is classified as a label, procedure, description, machine or
      technical text, or technical term.
- [ ] Customer terms use the approved spelling, capitalization, meaning, and
      plural or verb form.
- [ ] The text does not expose an internal Petri-net term as a public resource.
- [ ] Definitions match the architecture data model and the owning public
      contract.

### Sentence and paragraph shape

- [ ] Procedural sentences contain no more than 20 natural-language words.
- [ ] Each procedural sentence has one instruction unless simultaneous action
      is necessary.
- [ ] Procedures use imperative verbs, state necessary conditions first, and
      describe observable expected results.
- [ ] Sequence-dependent procedures use an ordered or vertical list.
- [ ] Descriptive sentences contain no more than 25 natural-language words.
- [ ] Descriptions introduce information gradually, keep one topic per
      paragraph, and use no more than six sentences per paragraph.
- [ ] Active voice is preferred, and every passive construction has a reason.
- [ ] Related information uses stable key words and phrases.

### Technical and contract fidelity

- [ ] Protected literals remain exact and come from the correct source.
- [ ] Natural-language comments remain understandable even when they are
      adjacent to protected code.
- [ ] Examples match shipped CLI behavior, API methods and routes, public
      schemas, identifiers, statuses, and error codes.
- [ ] Warnings identify the risk and affected object precisely.
- [ ] Destructive instructions state the consequence and required recovery.
- [ ] Failure guidance gives an observable symptom, a safe next action, and a
      way to confirm recovery.

### Rules, exceptions, and evidence

- [ ] Blocking deterministic findings are fixed or have a specific valid
      exception.
- [ ] Advisory findings were reviewed rather than blindly accepted or
      suppressed.
- [ ] Human reviewers confirmed meaning, subject-matter accuracy, logical
      order, safety, and contract fidelity.
- [ ] Each exception names its rule, exact scope, reason, owner, and review
      point.
- [ ] No exception hides ordinary readability debt or a contract mismatch.

The deterministic items can be supported by future automation. Vocabulary
meaning, technical accuracy, voice, information order, warnings, destructive
effects, recovery, and exception validity require language or subject-matter
judgment. This checklist does not describe future automation as implemented.

## Conformance and external references

The repository profile is **aligned with ASD-STE100 principles** for clear,
controlled technical English. It does not claim that this repository, any
document, or any future checker is certified, “ASD-STE100 compliant,” or fully
conformant. This profile does not reproduce or redistribute the ASD-STE100
controlled dictionary.

Use the [official ASD-STE100 site](https://www.asd-ste100.org/) for the current
standard and the [official ASD-STE100 FAQ](https://www.asd-ste100.org/faq.html)
for the distinction between writing rules, the controlled dictionary, and
software claims. Use the [official STE training guidance](https://www.asd-ste100.org/STE_training.html)
when planning author or reviewer training.

Before the repository can make a stronger project-specific claim, maintainers
MUST document all of the following:

- the exact ASD-STE100 issue and scope being adopted;
- use of the applicable writing rules and controlled dictionary, without
  treating this repository register as a replacement;
- training for authors and reviewers who apply the standard;
- human review by people with the required technical subject knowledge;
- owner-approved treatment of technical names, technical verbs, warnings,
  quoted output, generated artifacts, and exceptions; and
- evidence that the claim applies to the stated corpus and was reviewed against
  the official reference, not only against deterministic findings.

Until those conditions are documented and approved, use only the bounded
claim: **“The repository customer technical-writing profile is aligned with
ASD-STE100.”**
