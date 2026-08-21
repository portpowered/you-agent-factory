# Schema compatibility policy

This policy is the source-of-truth classification for additive compatibility
in customer-facing schemas. Runtime decoders apply the same boundary promise:
they validate known fields, accept one complete JSON document, ignore unknown
object fields, and report the ignored JSON paths without retaining their
values.

## Open boundaries

The following objects use `additionalProperties: true`:

- version-skewed customer configuration: the `Factory` document and its
  evolving authored topology, invocation, portability, resource, worker, and
  workstation objects; and `GlobalConfig` plus its defaults, runtime, model,
  price-table, worker-preset, and worker-settings objects;
- portable and canonical recording envelopes (`FactoryRecording`,
  `FactoryEvent`, and `FactoryEventContext`);
- evolving public Factory Event payload objects.

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

These objects remain closed because accepting an unrecognized command,
authorization value, discriminator branch, or fixed-shape field could change
execution or interpretation rather than merely add metadata. Each retained
closed schema must keep its rationale in this classification or in the schema
description.

The dashboard's tolerant consumption and presentation policy belongs to
compat-2. This backend policy does not add UI behavior. OpenAPI, Go, TypeScript,
and packaged schema artifacts are generated from authored sources with the
repository generation targets; generated files are never hand-edited.
