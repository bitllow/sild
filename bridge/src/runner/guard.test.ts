import { after, before, describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { decide, type GuardConfig } from './guard.ts';

let base: string;
let cfg: GuardConfig;

before(() => {
  base = mkdtempSync(path.join(tmpdir(), 'guard-'));
  const src = path.join(base, 'src');
  const work = path.join(base, 'work', 'c1');
  mkdirSync(path.join(src, 'app'), { recursive: true });
  mkdirSync(work, { recursive: true });
  mkdirSync(path.join(base, 'work', 'c2'), { recursive: true });
  mkdirSync(path.join(base, 'work', '.cfg', 'c1'), { recursive: true });
  writeFileSync(path.join(src, 'app', 'main.txt'), 'code');
  writeFileSync(path.join(work, 'data.json'), '{}');
  writeFileSync(path.join(base, 'work', 'c2', 'other.json'), '{}');
  writeFileSync(path.join(base, 'secret.txt'), 'secret');
  symlinkSync(path.join(base, 'secret.txt'), path.join(src, 'link-out'));
  symlinkSync(path.join(base, 'work', 'c2'), path.join(work, 'sibling'));
  cfg = {
    tools: new Set(['Read', 'Grep', 'Glob', 'mcp__ops__reply']),
    roots: [src, work],
    cwd: work,
  };
});

after(() => rmSync(base, { recursive: true, force: true }));

const p = (...parts: string[]) => path.join(base, ...parts);

describe('tool allowlist', () => {
  test('allows a listed MCP tool', () => {
    assert.deepEqual(decide(cfg, 'mcp__ops__reply', { text: 'hi' }), { allow: true });
  });
  for (const name of ['Bash', 'Write', 'Edit', 'WebFetch', 'Agent', 'mcp__ops__forbidden_probe', 'mcp__evil__x']) {
    test(`denies ${name}`, () => assert.equal(decide(cfg, name, {}).allow, false));
  }
});

describe('Read', () => {
  test('allows a file in the checkout', () => {
    assert.equal(decide(cfg, 'Read', { file_path: p('src', 'app', 'main.txt') }).allow, true);
  });
  test('allows a file in the conversation workspace, relative to cwd', () => {
    assert.equal(decide(cfg, 'Read', { file_path: 'data.json' }).allow, true);
  });
  test('denies /proc/self/environ', () => {
    assert.equal(decide(cfg, 'Read', { file_path: '/proc/self/environ' }).allow, false);
  });
  test('denies /etc/passwd', () => {
    assert.equal(decide(cfg, 'Read', { file_path: '/etc/passwd' }).allow, false);
  });
  test('denies a symlink that points out of the checkout', () => {
    assert.equal(decide(cfg, 'Read', { file_path: p('src', 'link-out') }).allow, false);
  });
  test('denies another conversation through a symlink', () => {
    assert.equal(decide(cfg, 'Read', { file_path: p('work', 'c1', 'sibling', 'other.json') }).allow, false);
  });
  test('denies the conversation config dir', () => {
    assert.equal(decide(cfg, 'Read', { file_path: p('work', '.cfg', 'c1') }).allow, false);
  });
  test('denies traversal out of the workspace', () => {
    assert.equal(decide(cfg, 'Read', { file_path: '../c2/other.json' }).allow, false);
  });
  test('denies a missing file_path', () => {
    assert.equal(decide(cfg, 'Read', {}).allow, false);
  });
});

describe('Grep', () => {
  test('defaults to cwd', () => {
    assert.equal(decide(cfg, 'Grep', { pattern: 'x' }).allow, true);
  });
  test('allows the checkout', () => {
    assert.equal(decide(cfg, 'Grep', { pattern: 'x', path: p('src') }).allow, true);
  });
  test('denies a parent path', () => {
    assert.equal(decide(cfg, 'Grep', { pattern: 'x', path: '..' }).allow, false);
  });
  test('denies /', () => {
    assert.equal(decide(cfg, 'Grep', { pattern: 'x', path: '/' }).allow, false);
  });
  test('denies a glob that climbs out', () => {
    assert.equal(decide(cfg, 'Grep', { pattern: 'x', glob: '../**' }).allow, false);
  });
  test('denies an absolute glob', () => {
    assert.equal(decide(cfg, 'Grep', { pattern: 'x', glob: '/etc/*' }).allow, false);
  });
});

describe('Glob', () => {
  test('allows a relative pattern in cwd', () => {
    assert.equal(decide(cfg, 'Glob', { pattern: '**/*.json' }).allow, true);
  });
  test('allows an absolute pattern under the checkout', () => {
    assert.equal(decide(cfg, 'Glob', { pattern: `${p('src')}/**/*.txt` }).allow, true);
  });
  test('denies /**', () => {
    assert.equal(decide(cfg, 'Glob', { pattern: '/**' }).allow, false);
  });
  test('denies an absolute pattern outside', () => {
    assert.equal(decide(cfg, 'Glob', { pattern: '/etc/*' }).allow, false);
  });
  test('denies .. in the pattern', () => {
    assert.equal(decide(cfg, 'Glob', { pattern: '../**' }).allow, false);
  });
  test('denies a path outside', () => {
    assert.equal(decide(cfg, 'Glob', { pattern: '*', path: '/etc' }).allow, false);
  });
});
