import { expect, test } from 'bun:test';
import { EventEmitter } from 'node:events';
import { runPhase } from './run-phase.mjs';

function fixture(finish) {
  const child = new EventEmitter();
  const signals = new EventEmitter();
  const records = [];
  const calls = [];
  let tick = 0;
  child.kill = (signal) => { calls.push(signal); return true; };
  return { child, signals, records, calls, options: {
    now: () => tick++ * 5,
    log: (record) => records.push(record),
    signals,
    spawnChild: (...args) => {
      calls.push(args);
      queueMicrotask(() => finish(child, signals));
      return child;
    },
  } };
}

for (const exitCode of [0, 7]) {
  test(`preserves exit ${exitCode}, argument boundaries and elapsed diagnostics`, async () => {
    const f = fixture((child) => child.emit('close', exitCode, null));
    expect(await runPhase('build', ['tool', 'a b', '$(literal)'], f.options)).toBe(exitCode);
    expect(f.calls).toEqual([['tool', ['a b', '$(literal)'], { stdio: 'inherit', shell: false }]]);
    expect(f.records).toEqual([
      { phase: 'build', event: 'start', elapsedMs: 5, command: ['tool', 'a b', '$(literal)'] },
      { phase: 'build', event: exitCode ? 'error' : 'end', elapsedMs: 10, exitCode, signal: null },
    ]);
  });
}

test('spawn error joins child close and reports failure', async () => {
  const f = fixture((child) => {
    child.emit('error', new Error('ENOENT'));
    expect(f.records).toHaveLength(1);
    child.emit('close', -2, null);
  });
  expect(await runPhase('build', ['missing'], f.options)).toBe(1);
  expect(f.records[1]).toMatchObject({ event: 'error', error: 'ENOENT', exitCode: 1 });
});

test('synchronous spawn error is diagnostic and nonzero', async () => {
  const f = fixture(() => {});
  f.options.spawnChild = () => { throw new Error('spawn refused'); };
  expect(await runPhase('build', ['tool'], f.options)).toBe(1);
  expect(f.records[1]).toMatchObject({ event: 'error', error: 'spawn refused' });
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  test(`forwards ${signal}, joins close and removes owned listeners`, async () => {
    const f = fixture((child, signals) => {
      signals.emit(signal);
      expect(f.records).toHaveLength(1);
      child.emit('close', null, signal);
    });
    const peer = () => {};
    f.signals.on(signal, peer);
    expect(await runPhase('build', ['tool'], f.options)).toBe(signal === 'SIGINT' ? 130 : 143);
    expect(f.calls[1]).toBe(signal);
    expect(f.signals.listeners(signal)).toEqual([peer]);
    expect(f.records[1]).toMatchObject({ event: 'error', signal });
  });
}

test('invalid invocation never starts a child', async () => {
  const f = fixture(() => {});
  await expect(runPhase('build', [], f.options)).rejects.toThrow('usage');
  expect(f.calls).toEqual([]);
});

test('signal forwarding failure remains nonzero even if child exits zero', async () => {
  const f = fixture((child, signals) => {
    signals.emit('SIGTERM');
    child.emit('close', 0, null);
  });
  f.child.kill = () => false;
  expect(await runPhase('build', ['tool'], f.options)).toBe(1);
  expect(f.records[1]).toMatchObject({ event: 'error', error: 'could not forward SIGTERM' });
});
