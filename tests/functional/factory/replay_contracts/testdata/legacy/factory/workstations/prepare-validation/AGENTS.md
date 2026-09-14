# Prepare independent validation

Run the configured preparation script. It admits only mission fields, rejects
reused directories, verifies artifact hashes while staging private copies, and
records private child-process environment settings in mission.json. A failed
preparation requires a new validation Work name after correction. It cannot
claim customer acceptance or edit the project contract.

New missions must include the strict `factory-preflight.v1` envelope used by
`setup-workspace.py`: project identity and contract revision, three hash-pinned
authority files, and an intended-mainline commit. Preparation verifies those
inputs and all build, fixture, and public-document descriptors before creating
the probe directory. The private mission record keeps verified identities and
the resolved checkout head, but does not persist authority paths or contents.
Retrospective missions may omit the build; customer and engineering missions
may not. Failed post-target staging leaves recoverable evidence without a
`mission.json` ready record, and the next attempt must use a fresh name.
