# Connect the generic Cobra constructor to the application root

The schema-generic Cobra constructor currently has package-level behavioral
coverage but is not reachable through `root.BuildProcess`. Direct construction
under `tests/functional` violates the application-test boundary, while removing
those scenarios drops the existing `climanifestcobra` functional coverage floor
because the production process still uses family constructors.

Plan the command-family migration that supplies stable handler and completion
registries to the generic constructor through canonical Wire composition.
Preserve existing customer CLI behavior and move high-value generic projection
evidence to real `root.BuildProcess` invocations as each family becomes
reachable. Delete direct-constructor tests from `tests/functional` once the
boundary-compliant flows restore the functional coverage floor; keep detailed
schema projection cases beside the owning package.
