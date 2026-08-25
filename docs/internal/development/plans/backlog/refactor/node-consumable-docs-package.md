# problem statement

The canonical customer documentation is exposed through `you docs`, but Node
consumers cannot install the same documentation as a versioned package for use
by a future static site or other build-time tooling.

## customer ask

Publish the existing packaged CLI documentation as an npm package so a Node
repository can consume it without copying or separately maintaining the
Markdown.

## solution

Create a data-only package at `packages/docs` named
`@you-agent-factory/docs`. The package will publish the exact canonical topic
surface exposed by `you docs`, together with a generated manifest containing
topic order, descriptions, aliases, artifact paths, and content digests.

Keep `docs/reference/*.md` authoritative for content and keep the existing CLI
topic registry in `pkg/transports/cli/docs` authoritative for exposure, order,
descriptions, and aliases. Generate the npm publication projection from the
registry's `TopicIndexEntries()` and `Markdown()` operations so packaged topic
content remains byte-equivalent to `you docs <topic>`.

The initial package is a framework-neutral build-time input. It will not own a
Markdown renderer, React components, browser state, static-site routing, or
link rewriting. A later static site can resolve and read the manifest and raw
Markdown through stable npm package exports.

# original document

The customer-visible and planning references for this work are:

- `C:\Users\andre\work\portos\infinite-you\docs\reference\README.md`
- `C:\Users\andre\work\portos\infinite-you\docs\reference\embed.go`
- `C:\Users\andre\work\portos\infinite-you\pkg\transports\cli\docs\docs.go`
- `factory/docs/standards/planning-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\packaged-structure.md`

# changes

## package changes

### Public package shape

Add `packages/docs` with the following authored and generated boundaries:

```text
packages/docs/
  package.json
  README.md
  LICENSE.md
  generated/
    manifest.json
    topics/
      <canonical-topic>.md
```

The proposed package identity is `@you-agent-factory/docs`. Its publication
allowlist must contain only the package metadata, maintainer README, license,
generated manifest, and canonical generated topic files. It must have no
runtime dependencies, executable entrypoint, lifecycle scripts, or browser
assumptions.

The proposed public exports are:

```json
{
  "./manifest": "./generated/manifest.json",
  "./topics/*": "./generated/topics/*.md"
}
```

Canonical topic names are stable package subpaths. Compatibility aliases are
manifest metadata that point consumers to canonical topics; they do not create
duplicate Markdown artifacts.

### Generated publication projection

Add a focused generator, such as `cmd/docsreferencepackagegenerate`, that reads
the existing CLI docs registry through its public operations and writes the
deterministic package projection. The generator must support both generation
and drift-check modes.

Add Make targets with these responsibilities:

- `docs-package-generate` refreshes the generated manifest and topic files.
- `docs-package-check` fails when the checked-in projection differs from the
  canonical CLI docs surface.
- `docs-package-verify` runs generation drift, package contract, tarball
  inventory, and installed-consumer verification.

Include docs package generation in `build-all`. Include projection drift in
`docs-reference-smoke` so a maintainer editing canonical Markdown receives an
actionable failure without needing to know the npm publication topology.

Update `docs/reference/README.md`, `docs/README.md`, and
`docs/architecture/packaged-structure.md` to describe the generated npm
projection while retaining `docs/reference/` as the only authored content
tree.

### Candidate and publication flow

Follow the existing root data-package candidate pattern used by
`@you-agent-factory/api` and `@you-agent-factory/packaged-factories`:

- add focused docs-package pack, contract, consumer, candidate, registry,
  dry-run, and publication helpers or parameterize an existing shared helper
  where that keeps the resulting behavior simpler;
- build a no-publish candidate on relevant pull requests;
- add `@you-agent-factory/docs` to the complete tagged-release package set;
- preserve the reviewed tarball and candidate evidence between preparation
  and publication;
- reconcile immutable registry versions and verify the installed published
  package after publication.

Update `cmd/ciclassify` so changes under `docs/reference/`, `packages/docs/`,
the docs generator, and docs-package release scripts select the docs reference
and docs package verification lanes as applicable. Update CI's verification
policy so a skipped, failed, or cancelled required docs package lane cannot be
reported as successful.

## contracts

Introduce the following public npm contract:

- package name: `@you-agent-factory/docs`;
- stable manifest export: `@you-agent-factory/docs/manifest`;
- stable topic exports:
  `@you-agent-factory/docs/topics/<canonical-topic>`;
- manifest format: `you-agent-factory.docs/v1`;
- topic content: raw Markdown byte-equivalent to the canonical
  `you docs <topic>` result;
- alias behavior: manifest aliases identify their canonical topic and do not
  create independent content authorities.

The proposed manifest shape is:

```json
{
  "formatVersion": "you-agent-factory.docs/v1",
  "sourceCommit": "added to publication candidates",
  "topics": [
    {
      "name": "config",
      "description": "Operator initialization and Factory validation...",
      "aliases": [],
      "path": "generated/topics/config.md",
      "sha256": "..."
    }
  ]
}
```

The source projection may omit `sourceCommit`; candidate preparation must add
the reviewed source commit using the same provenance convention as the other
root data packages. Topic names, order, descriptions, aliases, paths, and
digests must be deterministic.

Removing or renaming a canonical topic, changing an export subpath, or changing
the manifest format is a public compatibility change. Supplemental files under
`docs/reference/` that are not exposed by `you docs`, including maintainer
indexes and non-topic pages, are outside the initial package contract.

## services

No product service is added or changed. The generator is repository tooling,
and the npm package is a publication projection. The Factory runtime, Factory
Sessions, Work, Workers, Providers, Recordings, Events, and other service
owners remain unchanged.

## API changes

There are no REST, OpenAPI, CLI command, MCP, ACP, or dashboard API changes.
The CLI's observable topic behavior must remain unchanged and serves as the
parity authority for the new package.

## tests

Add focused tests that prove:

- the manifest contains every canonical `you docs` topic exactly once and in
  the same display order;
- descriptions and aliases match `TopicIndexEntries()`;
- every generated topic is byte-equivalent to `Markdown(topic)`;
- every manifest digest matches its generated Markdown artifact;
- generation is deterministic and check mode reports added, removed, or
  changed artifacts clearly;
- `npm pack` contains the exact reviewed positive allowlist and every declared
  export target;
- an isolated Node project can install the real tarball, resolve the manifest,
  resolve every canonical topic through its package export, and read the raw
  Markdown;
- the isolated consumer can use alias metadata to select the canonical topic;
- candidate preparation records the requested version, source commit,
  inventory, contract digest, and tarball digest without mutating source
  package files;
- pull-request dry runs never publish;
- registry reconciliation rejects conflicting immutable versions and verifies
  an already-matching or newly published package;
- the tagged release candidate contains the exact complete public package set,
  including `@you-agent-factory/docs` once;
- CI classification selects both reference-doc and docs-package verification
  when canonical topics change, and selects the docs-package lane for
  package-only or release-helper changes.

# work stories

## story 1: install and read the exposed documentation

As a Node build-tool author, I can install `@you-agent-factory/docs`, discover
the supported topics, and read any canonical topic so I can use the same
documentation that ships in the CLI.

Acceptance criteria:

- A registry-format tarball installs successfully into an isolated Node
  project.
- `@you-agent-factory/docs/manifest` resolves to valid
  `you-agent-factory.docs/v1` JSON.
- Every canonical manifest entry resolves through
  `@you-agent-factory/docs/topics/<name>`.
- Every resolved Markdown artifact exactly matches `you docs <name>`.
- Topic order, descriptions, and aliases match the CLI topic index.
- Unsupported and supplemental repository documents are not accidentally
  exposed as canonical topic exports.

## story 2: edit documentation in one canonical location

As a documentation maintainer, I can edit the existing canonical Markdown or
CLI topic registry and deterministically regenerate the npm projection so I do
not maintain a second authored documentation tree.

Acceptance criteria:

- `docs-package-generate` produces the complete projection from the current
  CLI registry and embedded Markdown.
- Repeated generation produces no diff.
- `docs-package-check` succeeds on a current projection and fails with
  actionable paths when the projection is stale.
- Adding, removing, reordering, or renaming a CLI topic changes the manifest
  and generated inventory without a manually maintained second topic list.
- `docs-reference-smoke` exercises projection parity.

## story 3: review a real package candidate before publication

As a reviewer, I can rely on pull-request CI to build and consume the exact
docs tarball that would be published so package-boundary and generated-content
regressions block delivery.

Acceptance criteria:

- Relevant pull requests run `docs-package-verify` and build a no-publish
  candidate from the pull request head commit.
- Candidate evidence identifies the package, version, source commit, inventory,
  contract digest, and artifact digest.
- The candidate is installed and read from a clean Node consumer rather than
  through repository-relative paths or workspace links.
- CI classification and verification-policy tests prove that relevant changes
  cannot skip the required docs lanes.
- No pull-request path can invoke npm publication.

## story 4: publish and verify the reviewed package

As a Node consumer, I can install the docs package version associated with a
You Agent Factory release so documentation and CLI behavior are sourced from
the same reviewed commit.

Acceptance criteria:

- Tagged release preparation includes `@you-agent-factory/docs` exactly once at
  the release version.
- Publication validates preserved evidence and source commit before contacting
  the registry.
- Existing matching immutable versions are accepted only when their digest
  matches the candidate; conflicts fail safely.
- A newly published package is installed from the registry and passes the same
  manifest and topic consumer checks as the pull-request tarball.
- Publication changes do not alter the behavior or version alignment of the
  existing API, packaged Factory, or frontend packages.

# verification and delivery

The implementation should run the narrowest focused checks during development
and finish with at least:

```text
make docs-reference-smoke
make docs-package-verify
make public-release-package-smoke
go test ./cmd/docsreferencepackagegenerate/... ./pkg/transports/cli/docs/...
```

If `build-all`, CI classification, or tagged-release composition changes, run
their focused tests and the broader required PR verification tier as well.

The npm package name must be reserved and the repository's existing trusted
publication workflow must be configured as an npm trusted publisher for
`@you-agent-factory/docs`. This registry-side configuration is an external
delivery prerequisite.

Delivery is complete only after required CI is terminal and passing, all
blocking review feedback is addressed, merge conflicts are resolved, the
trusted-publisher prerequisite is confirmed, and the pull request is actually
merged. Opening a pull request, publishing a candidate, or reaching green CI
without merge is not completion.
