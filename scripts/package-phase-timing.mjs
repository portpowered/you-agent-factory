// Diagnostics stay on stderr so caller-facing JSON remains unchanged.
export async function timePackagePhase(
  label,
  operation,
  { now = Date.now, log = (message) => console.error(message) } = {},
) {
  const started = now();
  log(`[package-phase] ${label} start`);
  let outcome = "failed";
  try {
    const result = await operation();
    outcome = "passed";
    return result;
  } finally {
    log(`[package-phase] ${label} ${outcome} elapsed_ms=${now() - started}`);
  }
}
