# Release Smoke Fixture

This fixture is the canonical packaged-binary smoke target for release verification.

- It is intentionally small and self-contained.
- It includes one checked-in seed input so the packaged CLI can process work immediately in continuous service mode.
- Release smoke workflows should point the harness at this directory instead of inventing per-workflow fixtures.
