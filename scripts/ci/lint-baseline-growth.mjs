import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function parseBaseline(text) {
  const keys = new Set();
  for (const raw of text.split(/\r?\n/u)) {
    const key = raw.trim();
    if (!key || key.startsWith('#')) continue;
    const parts = key.split('|');
    if (parts.length !== 3 || parts.some(part => !part || part.trim() !== part)) {
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

export function compareBaselines(baseText, headText) {
  const base = parseBaseline(baseText);
  const head = parseBaseline(headText);
  const established = new Set([...base].map(key => key.split('|')[0]));
  return [...head].filter(key => established.has(key.split('|')[0]) && !base.has(key)).sort();
}

// Compiler metadata alone identifies vanished units; no source inventory is read.
export function orphanTimingKeys(headText, unitText, prefix) {
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
  return [...parseBaseline(headText)].filter(key => {
    const [rule, unit, site] = key.split('|');
    const file = site.split('::')[0].split('/').at(-1);
    return rule.startsWith('testsleep-') &&
      (!units.has(unit) || (rule.endsWith('-test') && !units.get(unit)) ||
       (files.has(unit) && !files.get(unit).has(file)));
  }).sort();
}

function main(args) {
  const options = Object.fromEntries(Array.from({ length: args.length / 2 }, (_, i) => [args[2 * i], args[2 * i + 1]]));
  if (args.length % 2 || !options['--head'] || (!options['--base'] && !options['--units'])) {
    throw new Error('usage: --head FILE --base FILE, or --head FILE --units FILE --module-prefix PREFIX');
  }
  const head = readFileSync(options['--head'], 'utf8');
  const failures = options['--units']
    ? orphanTimingKeys(head, readFileSync(options['--units'], 'utf8'), options['--module-prefix'] ?? 'github.com/portpowered/infinite-you/')
    : compareBaselines(readFileSync(options['--base'], 'utf8'), head);
  if (failures.length) throw new Error(`lint baseline rejected keys:\n${failures.join('\n')}`);
  console.log('lint baseline: established rules did not grow; timing owners are valid when checked');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main(process.argv.slice(2)); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
