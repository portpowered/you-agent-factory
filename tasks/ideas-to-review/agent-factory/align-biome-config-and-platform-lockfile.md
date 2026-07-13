# Align Biome configuration and platform lockfile entries

## Problem

`make lint` cannot reach repository lint checks with the currently pinned UI
toolchain. The lockfile does not install the Windows Biome binary through
`npm ci`, and after installing the matching `@biomejs/cli-win32-x64@2.4.14`
package, Biome rejects the checked-in `noExcessiveLinesPerFile` configuration
because that rule is no longer present in Biome 2.4.

This blocks unrelated backend work from running the required aggregate lint
gate and is likely to recur across Windows worktrees.

## Suggested outcome

- Pin a Biome version compatible with the authored configuration or migrate the
  configuration to supported 2.4 rules without weakening the repository's file
  size policy.
- Regenerate the npm lockfile with required supported-platform optional binary
  entries so a clean Windows `npm ci` provides the Biome executable.
- Verify clean installs and `make lint` on Windows and the primary CI platform.
