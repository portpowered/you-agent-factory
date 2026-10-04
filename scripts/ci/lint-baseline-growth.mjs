import { readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

export function parseBaseline(text) {
  const keys = new Set();
  for (const raw of text.split(/\r?\n/u)) {
    const key = raw.trim();
    if (!key || key.startsWith('#')) continue;
    const parts = key.split('|');
    if (parts.length !== (parts[0] === 'petri-public' ? 4 : 3) || parts.some(part => !part || part.trim() !== part)) {
      throw new Error(`malformed baseline key: ${key}`);
    }
    if (keys.has(key)) throw new Error(`duplicate baseline key: ${key}`);
    if (parts[0].startsWith('testsleep-')) {
      const site = parts[2].split('::');
      if (!/^testsleep-(sleep|deadline|elapsed)(-test)?$/u.test(parts[0]) ||
          site.length !== 4 || !site[0].endsWith('.go') || !site[1] ||
          site[2] !== parts[0].replace('testsleep-', '').replace(/-test$/u, '') ||
          !/^[1-9]\d*$/u.test(site[3])) {
        throw new Error(`malformed timing baseline key: ${key}`);
      }
    }
    keys.add(key);
  }
  return keys;
}

// Compiler metadata alone identifies vanished units; no source inventory is read.
export function orphanTimingKeys(headText, unitText, prefix) {
  const timingKeys = [...parseBaseline(headText)].filter(key => key.startsWith('testsleep-'));
  if (!timingKeys.length) return [];
  const units = new Map();
  const files = new Map();
  for (const line of unitText.split(/\r?\n/u)) {
    const columns = line.trim().split('|');
    const [identity, tests = '', external = '', ignored = '', sources = ''] = columns;
    const path = identity.split(' [')[0];
    if (!path.startsWith(prefix) || path.endsWith('.test')) continue;
    const unit = path.slice(prefix.length);
    const testOwner = identity.includes(' [') || unit.endsWith('_test') || !!tests || !!external ||
      ignored.split(',').some(file => file.endsWith('_test.go'));
    units.set(unit, units.get(unit) || testOwner);
    if (columns.length > 1) {
      const names = files.get(unit) ?? new Set();
      for (const name of [tests, external, ignored, sources].join(',').split(',')) {
        if (name) names.add(name);
      }
      files.set(unit, names);
    }
  }
  if (!units.size) throw new Error('compiler unit metadata contains no repository units');
  return timingKeys.filter(key => {
    const [rule, unit, site] = key.split('|');
    const file = site.split('::')[0].split('/').at(-1);
    return rule.startsWith('testsleep-') &&
      (!units.has(unit) || (rule.endsWith('-test') && !units.get(unit)) ||
       (files.has(unit) && !files.get(unit).has(file)));
  }).sort();
}

// Query only debt-owning directories, then resolve platform-inactive owners on
// the other supported GOOS values. IgnoredGoFiles includes default-only files,
// so tagged metadata suffices for ownership (both vet configurations still run).
export function collectTimingUnits(headText, tags, go = 'go', run = spawnSync, host = process.platform) {
  const keys = [...parseBaseline(headText)].filter(key => key.startsWith('testsleep-'));
  const platforms = [...new Set([host === 'win32' ? 'windows' : host, 'linux', 'windows', 'darwin'])];
  const template = '{{.ImportPath}}|{{join .TestGoFiles ","}}|{{join .XTestGoFiles ","}}|{{join .IgnoredGoFiles ","}}|{{join .GoFiles ","}}';
  let metadata = '';
  let unresolved = keys;
  for (const goos of platforms) {
    if (!unresolved.length) break;
    // Use the repository-file directory, not a guessed _test suffix removal:
    // real directory names may also end in _test.
    const packages = [...new Set(unresolved.map(key => './' + key.split('|')[2].split('::')[0].split('/').slice(0, -1).join('/')))];
    const result = run(go, ['list', '-e', '-test', `-tags=${tags}`, '-f', template, ...packages], {
      encoding: 'utf8', maxBuffer: 32 * 1024 * 1024,
      env: { ...process.env, GOOS: goos },
    });
    if (result.error || result.status !== 0) {
      throw new Error(`compiler metadata (${goos}) failed: ${result.error?.message ?? result.stderr}`);
    }
    metadata += result.stdout + '\n';
    unresolved = orphanTimingKeys(headText, metadata, 'github.com/portpowered/infinite-you/');
  }
  return metadata;
}

function main(args) {
  const options = Object.fromEntries(Array.from({ length: args.length / 2 }, (_, i) => [args[2 * i], args[2 * i + 1]]));
  if (args.length % 2 || !options['--head'] || !options['--units']) {
    throw new Error('usage: --head FILE --units FILE [--collect-tags TAGS --go EXECUTABLE]');
  }
  const head = readFileSync(options['--head'], 'utf8');
  if (options['--collect-tags']) {
    writeFileSync(options['--units'], collectTimingUnits(head, options['--collect-tags'], options['--go']));
  }
  const failures = orphanTimingKeys(head, readFileSync(options['--units'], 'utf8'), options['--module-prefix'] ?? 'github.com/portpowered/infinite-you/');
  if (failures.length) throw new Error(`lint baseline rejected keys:\n${failures.join('\n')}`);
  console.log('lint timing ownership: owners and files are valid');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main(process.argv.slice(2)); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
