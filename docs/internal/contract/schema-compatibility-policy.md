# Schema compatibility policy

This policy is the source-of-truth classification for additive compatibility
in customer-facing schemas. Runtime decoders validate known fields, accept one
complete JSON document, and report ignored JSON paths without retaining their
values. Published schemas remain closed where an unknown field could change an
execution or interpretation contract.

## Open boundaries

The following objects use `additionalProperties: true`:

- version-skewed operator configuration: `GlobalConfig` plus its defaults,
  runtime, model, price-table, worker-preset, and worker-settings objects;
- additive metadata-bearing Factory children whose fields do not change the
  execution contract, including invocation, portability, worker, and
  workstation metadata objects;
- selected additive event payloads, including run-request and session-start
  payloads; the fixed `FactoryEventContext` identity and ordering envelope
  remains closed.

The customer-facing Factory runtime mapper is independently tolerant at its
decode boundary and reports ignored paths, while the published Factory schema
keeps its root and fixed execution-shape objects closed. This prevents a
packaged-factory consumer or validation ratchet from silently treating an
unknown top-level definition as executable configuration.

Open schema properties are compatibility-only. A future value in a known
field is still rejected when that field violates its type, enum, required-field,
or other documented constraint. Unknown fields are never used to bypass
runtime validation, and exact-one-document checks remain strict.
Open objects may also carry targeted `not` exclusions for documented retired
fields, such as `workstations[].join`; those exclusions preserve the retired
field rejection without closing the rest of the evolving object.

## Intentionally closed boundaries

`additionalProperties: false` remains appropriate for contracts whose shape is
itself a safety or interpretation boundary. The current closed classifications
are:

- command and authorization inputs, including ACP launch settings, hosted
  worker authentication, tool permissions, and lifecycle-control payloads;
- discriminator-selected variants and their fixed-shape geometry, including
  orchestrator variants and Factory Layout primitives;
- fixed public projections and identity-only event references. In particular,
  `DispatchWorkerSessionAssociationEventPayload` intentionally serves only
  `workerSessionId`; canonical recording data may retain execution-only
  `model` and `reasoningEffort`, but the public projection does not expose them;
- fixed-shape members of the public event payload `oneOf` union when their
  closed field set is needed to keep event-shape validation unambiguous. An
  event payload is opened when it is classified as an additive extension
  boundary; this does not turn retired fields, control operations, or identity
  projections into accepted aliases;
- repository-owned inventories, generated manifests, diagnostics, and other
  fixed semantic read models.

Factory, Work, WorkType, Guard, Resource, FactoryEvent, and FactoryRecording
are also intentionally closed at their published roots. Their runtime-owned
compatibility decoders may still report additive fields without using them;
the closed schemas keep package consumers and repository ratchets from
accepting unknown execution semantics.

These objects remain closed because accepting an unrecognized command,
authorization value, discriminator branch, or fixed-shape field could change
execution or interpretation rather than merely add metadata. Each retained
closed schema must keep its rationale in this classification or in the schema
description.

The dashboard's tolerant consumption and presentation policy belongs to
compat-2. This backend policy does not add UI behavior. OpenAPI, Go, TypeScript,
and packaged schema artifacts are generated from authored sources with the
repository generation targets; generated files are never hand-edited.
