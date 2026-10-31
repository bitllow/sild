// The JSON Schema subset catalog params may use. Anything outside it is
// refused when the catalog is loaded rather than silently not enforced.

const TYPES = new Set(['string', 'number', 'integer', 'boolean', 'object']);
const VALIDATING = new Set(['type', 'required', 'properties', 'additionalProperties', 'enum', 'minimum', 'maximum', 'pattern']);
const ANNOTATIONS = new Set(['$schema', 'title', 'description', 'examples', 'default', '$comment']);

type Schema = Record<string, unknown>;

// Errors in the schema itself; empty when it stays inside the subset.
export function checkSchema(schema: unknown, at = 'params'): string[] {
  if (!isRecord(schema)) return [`${at}: schema must be an object`];
  const errors: string[] = [];
  for (const key of Object.keys(schema)) {
    if (!VALIDATING.has(key) && !ANNOTATIONS.has(key)) errors.push(`${at}: unsupported keyword ${key}`);
  }
  if (schema.type !== undefined && !(typeof schema.type === 'string' && TYPES.has(schema.type))) {
    errors.push(`${at}: unsupported type ${JSON.stringify(schema.type)}`);
  }
  if (schema.required !== undefined && !(Array.isArray(schema.required) && schema.required.every((r) => typeof r === 'string'))) {
    errors.push(`${at}: required must be a list of names`);
  }
  if (schema.enum !== undefined && !Array.isArray(schema.enum)) errors.push(`${at}: enum must be a list`);
  for (const k of ['minimum', 'maximum'] as const) {
    if (schema[k] !== undefined && typeof schema[k] !== 'number') errors.push(`${at}: ${k} must be a number`);
  }
  if (schema.additionalProperties !== undefined && typeof schema.additionalProperties !== 'boolean') {
    errors.push(`${at}: additionalProperties must be a boolean`);
  }
  if (schema.pattern !== undefined) {
    try {
      new RegExp(String(schema.pattern), 'u');
    } catch {
      errors.push(`${at}: bad pattern`);
    }
  }
  if (schema.properties !== undefined) {
    if (!isRecord(schema.properties)) errors.push(`${at}: properties must be an object`);
    else for (const [k, sub] of Object.entries(schema.properties)) errors.push(...checkSchema(sub, `${at}.${k}`));
  }
  return errors;
}

// Errors in value against schema; empty when it validates.
export function validate(schema: Schema, value: unknown, at = 'params'): string[] {
  const errors: string[] = [];
  const type = schema.type;
  if (typeof type === 'string' && !hasType(type, value)) return [`${at}: expected ${type}`];

  if (Array.isArray(schema.enum) && !schema.enum.some((e) => e === value)) errors.push(`${at}: not one of the allowed values`);
  if (typeof value === 'number') {
    if (typeof schema.minimum === 'number' && value < schema.minimum) errors.push(`${at}: below ${schema.minimum}`);
    if (typeof schema.maximum === 'number' && value > schema.maximum) errors.push(`${at}: above ${schema.maximum}`);
  }
  if (typeof value === 'string' && typeof schema.pattern === 'string' && !new RegExp(schema.pattern, 'u').test(value)) {
    errors.push(`${at}: does not match ${schema.pattern}`);
  }
  if (isRecord(value)) {
    const props = isRecord(schema.properties) ? schema.properties : {};
    for (const name of Array.isArray(schema.required) ? schema.required : []) {
      if (!(String(name) in value)) errors.push(`${at}.${name}: required`);
    }
    for (const [k, v] of Object.entries(value)) {
      const sub = props[k];
      if (isRecord(sub)) errors.push(...validate(sub, v, `${at}.${k}`));
      else if (schema.additionalProperties === false) errors.push(`${at}.${k}: not allowed`);
    }
  }
  return errors;
}

function hasType(type: string, v: unknown): boolean {
  switch (type) {
    case 'string':
      return typeof v === 'string';
    case 'number':
      return typeof v === 'number' && Number.isFinite(v);
    case 'integer':
      return Number.isInteger(v);
    case 'boolean':
      return typeof v === 'boolean';
    case 'object':
      return isRecord(v);
  }
  return false;
}

export function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}
