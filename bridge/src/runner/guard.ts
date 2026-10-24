import { realpathSync } from 'node:fs';
import path from 'node:path';

export type GuardConfig = {
  // Tool names the session may call; everything else is denied.
  tools: ReadonlySet<string>;
  // Directories file tools may touch. Compared after resolving symlinks.
  roots: readonly string[];
  // The session's working directory, which Grep/Glob default to.
  cwd: string;
};

export type Decision = { allow: true } | { allow: false; reason: string };

const FILE_TOOLS = new Set(['Read', 'Grep', 'Glob']);
const GLOB_CHARS = /[*?[{]/;

const deny = (reason: string): Decision => ({ allow: false, reason });

export function decide(cfg: GuardConfig, toolName: string, input: Record<string, unknown>): Decision {
  if (!cfg.tools.has(toolName)) return deny(`tool ${toolName} is not available`);
  if (!FILE_TOOLS.has(toolName)) return { allow: true };

  const roots = cfg.roots.map(real).filter((r): r is string => r !== null);
  const inside = (p: string): boolean => {
    const resolved = real(path.resolve(cfg.cwd, p));
    return resolved !== null && roots.some((root) => contains(root, resolved));
  };

  switch (toolName) {
    case 'Read': {
      const file = input.file_path;
      if (typeof file !== 'string' || !inside(file)) return deny('path is outside the allowed directories');
      return { allow: true };
    }
    case 'Grep': {
      const dir = typeof input.path === 'string' ? input.path : cfg.cwd;
      if (!inside(dir)) return deny('path is outside the allowed directories');
      if (typeof input.glob === 'string' && escapes(input.glob)) return deny('glob may not leave the search path');
      return { allow: true };
    }
    case 'Glob': {
      const dir = typeof input.path === 'string' ? input.path : cfg.cwd;
      if (!inside(dir)) return deny('path is outside the allowed directories');
      const pattern = typeof input.pattern === 'string' ? input.pattern : '';
      if (hasParentSegment(pattern)) return deny('pattern may not contain ..');
      // An absolute pattern searches from its own static prefix, not from path.
      if (path.isAbsolute(pattern) && !inside(staticPrefix(pattern))) {
        return deny('pattern is outside the allowed directories');
      }
      return { allow: true };
    }
  }
  return deny(`tool ${toolName} is not available`);
}

// Fails closed: a path that does not resolve is treated as outside.
function real(p: string): string | null {
  try {
    return realpathSync(p);
  } catch {
    return null;
  }
}

function contains(root: string, p: string): boolean {
  const rel = path.relative(root, p);
  return rel === '' || (!rel.startsWith('..') && !path.isAbsolute(rel));
}

function hasParentSegment(pattern: string): boolean {
  return pattern.split(/[\\/]/).includes('..');
}

function escapes(glob: string): boolean {
  return path.isAbsolute(glob) || hasParentSegment(glob);
}

function staticPrefix(pattern: string): string {
  const segments = pattern.split('/');
  const firstGlob = segments.findIndex((s) => GLOB_CHARS.test(s));
  const fixed = firstGlob === -1 ? segments : segments.slice(0, firstGlob);
  return fixed.join('/') || '/';
}
