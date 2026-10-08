import { spawn } from 'node:child_process';

// Keep the child output and argument boundaries intact; diagnostics use stderr.
export async function runPhase(phase, command, {
  spawnChild = spawn,
  now = () => performance.now(),
  log = (record) => process.stderr.write(`${JSON.stringify(record)}\n`),
  signals = process,
} = {}) {
  if (!phase || !command.length || command.some((arg) => typeof arg !== 'string')) {
    throw new Error('usage: run-phase.mjs <phase> -- <command> [argument ...]');
  }
  const started = now();
  const emit = (event, details = {}) => log({
    phase, event, elapsedMs: Math.max(0, now() - started), ...details,
  });
  emit('start', { command });
  let child;
  try {
    child = spawnChild(command[0], command.slice(1), { stdio: 'inherit', shell: false });
  } catch (error) {
    emit('error', { error: error.message, exitCode: 1, signal: null });
    return 1;
  }
  let childError;
  let forwardedSignal;
  const forward = (signal) => {
    forwardedSignal = signal;
    try {
      if (!child.kill(signal)) childError = new Error(`could not forward ${signal}`);
    } catch (error) {
      childError = error;
    }
  };
  const interrupt = () => forward('SIGINT');
  const terminate = () => forward('SIGTERM');
  signals.on('SIGINT', interrupt);
  signals.on('SIGTERM', terminate);
  try {
    // An error precedes close for spawn failures. Join close before returning.
    const result = await new Promise((resolve) => {
      child.on('error', (error) => { childError = error; });
      child.once('close', (exitCode, signal) => resolve({ exitCode, signal }));
    });
    const signal = result.signal || forwardedSignal || null;
    const exitCode = childError ? 1 : signal ? (signal === 'SIGINT' ? 130 : 143) : (result.exitCode ?? 1);
    emit(exitCode === 0 ? 'end' : 'error', {
      exitCode, signal, ...(childError ? { error: childError.message } : {}),
    });
    return exitCode;
  } finally {
    signals.removeListener('SIGINT', interrupt);
    signals.removeListener('SIGTERM', terminate);
  }
}

if (import.meta.main) {
  const [phase, separator, ...command] = process.argv.slice(2);
  try {
    if (separator !== '--') throw new Error('expected -- before command');
    process.exitCode = await runPhase(phase, command);
  } catch (error) {
    process.stderr.write(`${JSON.stringify({ phase, event: 'error', error: error.message, exitCode: 1 })}\n`);
    process.exitCode = 1;
  }
}
