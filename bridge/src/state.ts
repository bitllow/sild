import { mkdirSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';

const KEY = /^[A-Za-z0-9_-]{1,128}$/;

// Small JSON documents that outlive the process: one file per key, replaced whole on each write.
export class JsonStore {
  #dir: string;

  constructor(dir: string) {
    mkdirSync(dir, { recursive: true });
    this.#dir = dir;
  }

  get<T>(key: string): T | undefined {
    try {
      return JSON.parse(readFileSync(this.#file(key), 'utf8')) as T;
    } catch {
      return undefined;
    }
  }

  set(key: string, value: unknown): void {
    const file = this.#file(key);
    writeFileSync(`${file}.tmp`, JSON.stringify(value));
    renameSync(`${file}.tmp`, file);
  }

  delete(key: string): void {
    rmSync(this.#file(key), { force: true });
  }

  keys(): string[] {
    return readdirSync(this.#dir).filter((f) => f.endsWith('.json')).map((f) => f.slice(0, -'.json'.length));
  }

  #file(key: string): string {
    if (!KEY.test(key)) throw new Error(`bad state key: ${JSON.stringify(key)}`);
    return path.join(this.#dir, `${key}.json`);
  }
}
