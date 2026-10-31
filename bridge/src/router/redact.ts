// Patterns, not the model, so a router failure cannot let phones or emails
// through to the session.

export type Found = { placeholder: string; kind: 'email' | 'phone'; value: string };

const EMAIL = /[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}/g;
// A bare digit run is an id (order, task, driver), not a phone: a phone has
// a leading + or 00, or separators between digit groups.
const PHONE = /(?<![\w.+-])(?:\+|00)?\d[\d ().-]{5,}\d(?![\w-])/g;
const DATE = /^\d{4}-\d{2}-\d{2}$|^\d{1,2}[./]\d{1,2}[./]\d{2,4}$/;

export function redact(text: string): { text: string; found: Found[] } {
  const found: Found[] = [];
  const counts = { email: 0, phone: 0 };
  const take = (kind: Found['kind'], value: string) => {
    const known = found.find((f) => f.kind === kind && f.value === value);
    if (known) return known.placeholder;
    const placeholder = `[${kind} ${++counts[kind]}]`;
    found.push({ placeholder, kind, value });
    return placeholder;
  };
  const out = text
    .replace(EMAIL, (m) => take('email', m))
    .replace(PHONE, (m) => (isPhone(m) ? take('phone', m) : m));
  return { text: out, found };
}

function isPhone(m: string): boolean {
  const digits = m.replace(/\D/g, '').length;
  if (digits < 7 || digits > 15 || DATE.test(m)) return false;
  return /^(\+|00)/.test(m) || /\d[ ().-]+\d/.test(m);
}
