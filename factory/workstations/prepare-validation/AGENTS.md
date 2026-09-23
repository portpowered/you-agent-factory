# Prepare independent validation

Run the configured preparation script. It admits mission fields, rejects reused
directories, verifies build, fixture, and public-document hashes while staging
private copies, and records private child-process environment settings in
mission.json. Project and Work identity come from the live Factory request;
missions do not duplicate them in a preflight envelope. A failed preparation
requires a new validation Work name after correction. It cannot claim customer
acceptance or edit the project contract.

Retrospective missions may omit the build; customer and engineering missions
may not. Failed post-target staging leaves recoverable evidence without a
`mission.json` ready record, and the next attempt must use a fresh name.
