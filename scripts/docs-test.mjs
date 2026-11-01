// Check repository Markdown links without network access or extra dependencies.
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { dirname, extname, isAbsolute, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const files = process.argv.length > 2 ? process.argv.slice(2)
  : execFileSync('git', ['ls-files', '-z', '--', '*.md'], { cwd: root, encoding: 'utf8' }).split('\0').filter(Boolean);
const errors = [];
const cache = new Map();
let checked = 0;
const load = file => {
  if (!cache.has(file)) cache.set(file, readFileSync(file, 'utf8'));
  return cache.get(file);
};
const headings = file => {
  const ids = new Set(), counts = new Map();
  let fence;
  for (const line of load(file).split(/\r?\n/)) {
    const marker = line.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (!fence) fence = marker[1][0];
      else if (fence === marker[1][0]) fence = undefined;
      continue;
    }
    if (fence) continue;
    const match = line.match(/^#{1,6}\s+(.+?)\s*#*\s*$/);
    if (!match) continue;
    const slug = match[1].replace(/<[^>]+>/g, '').toLowerCase()
      .replace(/[^\p{L}\p{M}\p{N}_\-\s]/gu, '').replace(/\s/g, '-');
    const count = counts.get(slug) || 0;
    counts.set(slug, count + 1);
    ids.add(slug + (count ? '-' + count : ''));
  }
  for (const match of load(file).matchAll(/(?:id|name)=["']([^"']+)["']/g)) ids.add(match[1]);
  return ids;
};

for (const name of files) {
  const file = resolve(root, name);
  const text = load(file).replace(/(^|\n)\s*(`{3,}|~{3,})[^\n]*\n[\s\S]*?\n\s*\2[^\n]*/g, '$1');
  const targets = [
    ...text.matchAll(/!?\[[^\]]*\]\(([^\s)]+)(?:\s+["'][^"']*["'])?\)/g),
    ...text.matchAll(/^\s*\[[^\]]+\]:\s*(\S+)/gm),
    ...text.matchAll(/(?:href|src)=["']([^"']+)["']/g),
  ].map(match => match[1]);
  for (const match of text.matchAll(/srcset=["']([^"']+)["']/g))
    targets.push(...match[1].split(',').map(item => item.trim().split(/\s+/)[0]));
  for (const raw of targets) {
    const target = raw.replace(/^<|>$/g, '');
    if (/^[a-z][a-z\d+.-]*:/i.test(target) || target.startsWith('//')) continue;
    checked++;
    const split = target.indexOf('#');
    const path = decodeURIComponent(split < 0 ? target : target.slice(0, split));
    const anchor = split < 0 ? '' : decodeURIComponent(target.slice(split + 1));
    const dest = path ? resolve(path.startsWith('/') ? root : dirname(file), path.startsWith('/') ? '.' + path : path) : file;
    const rel = relative(root, dest);
    if (rel === '..' || rel.startsWith('..' + sep) || isAbsolute(rel))
      errors.push(name + ': link leaves the repository: ' + raw);
    else if (!existsSync(dest))
      errors.push(name + ': missing path: ' + raw);
    else if (anchor && extname(dest) === '.md' && !headings(dest).has(anchor))
      errors.push(name + ': missing heading: ' + raw);
  }
}
if (errors.length) {
  console.error(errors.join('\n'));
  process.exitCode = 1;
} else {
  console.log('Documentation checks passed: ' + files.length + ' Markdown files, ' + checked + ' local links and images.');
}
