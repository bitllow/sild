import { redact } from './redact.ts';
import type { Proposal, RouteInput, RouteOutput, Router } from './router.ts';

// Deterministic keyword routing for tests and the local demo.
export type Rule = {
  source: string;
  when: RegExp;
  // Null when the text lacks a value the source needs.
  params: (text: string) => Record<string, unknown> | null;
  label: string | ((p: Record<string, unknown>) => string);
};

const NAME = /\b([A-ZÄÖÜÕŠŽ][a-zäöüõšž]+ [A-ZÄÖÜÕŠŽ][a-zäöüõšž]+)\b/u;
const DRIVER_ID = /\bdrv_\d+\b/;
const WEBHOOK_ID = /\bwh_[a-z0-9_]+\b/;
const TRIP = /\bTR-\d+\b/;
const VIN = /\b[A-HJ-NPR-Z0-9]{17}\b/;
const MONTHS = ['january', 'february', 'march', 'april', 'may', 'june', 'july', 'august', 'september', 'october', 'november', 'december'];

const LEAD = /\b(?:Driver|The|Our|My|Hi|Hello|Please)\s+/g;

function first(re: RegExp, text: string): string | null {
  const m = re.exec(text);
  return m ? (m[1] ?? m[0]) : null;
}

function month(text: string): string | null {
  const lower = text.toLowerCase();
  const i = MONTHS.findIndex((m) => lower.includes(m));
  if (i === -1) return null;
  const year = /\b(20\d\d)\b/.exec(text)?.[1] ?? '2026';
  return `${year}-${String(i + 1).padStart(2, '0')}`;
}

function driverQuery(text: string): string | null {
  const contact = redact(text).found[0];
  if (contact) return contact.value;
  return first(NAME, text.replace(LEAD, ''));
}

export const DEMO_RULES: Rule[] = [
  {
    source: 'driver.lookup',
    when: /\b(driver|trips?|app)\b/i,
    params: (t) => (DRIVER_ID.test(t) ? null : wrap('query', driverQuery(t))),
    label: (p) => `Looked up driver ${p.query}`,
  },
  {
    source: 'driver.devices',
    when: /\b(device|phone|token|notification|registration)s?\b/i,
    params: (t) => wrap('driver_id', first(DRIVER_ID, t)),
    label: 'Checked his phone registration',
  },
  {
    source: 'dispatch.pending',
    when: /\b(pending|waiting|trips?)\b/i,
    params: (t) => wrap('driver_id', first(DRIVER_ID, t)),
    label: 'Checked trips waiting for him',
  },
  {
    source: 'dispatch.available',
    when: /\b(reassign|available)\b/i,
    params: (t) => wrap('trip_id', first(TRIP, t)),
    label: 'Looked up available drivers near the pickup',
  },
  {
    source: 'webhook.deliveries',
    when: /\bwebhooks?\b/i,
    params: (t) => (WEBHOOK_ID.test(t) ? null : wrap('webhook', /\b(ERP|CRM|[A-Z]{3,})\b/.exec(t)?.[1] ?? null)),
    label: (p) => `Checked the ${p.webhook} webhook deliveries`,
  },
  {
    source: 'webhook.audit',
    when: /\b(audit|changes?|changed|secret|rotat\w*)\b/i,
    params: (t) => wrap('webhook_id', first(WEBHOOK_ID, t)),
    label: 'Checked recent changes to the webhook',
  },
  {
    source: 'vehicle.validate',
    when: /\bVIN\b/i,
    params: (t) => wrap('vin', first(VIN, t)),
    label: 'Checked the VIN format',
  },
  {
    source: 'vehicle.registry',
    when: /\bVIN\b/i,
    params: (t) => wrap('vin', first(VIN, t)),
    label: 'Looked up the VIN in the vehicle registry',
  },
  {
    source: 'invoices.export',
    when: /\b(invoice|export)\b/i,
    params: (t) => wrap('month', month(t)),
    label: (p) => `Opened the ${p.month} export`,
  },
  {
    source: 'orders.completed',
    when: /\b(orders?|dispatch report)\b/i,
    params: (t) => wrap('month', month(t)),
    label: 'Compared with orders completed that month',
  },
  {
    source: 'activity.errors',
    when: /\b(errors?|crash\w*)\b/i,
    params: () => ({ service: 'driver-app', days: 1 }),
    label: 'Checked recent errors',
  },
  {
    source: 'service.status',
    when: /\b(outage|down|status)\b/i,
    params: () => ({}),
    label: 'Checked service status',
  },
  {
    source: 'telematics.gps',
    when: /\b(gps|location|where)\b/i,
    params: (t) => wrap('vehicle', first(/\b\d{3}\s?[A-Z]{3}\b/, t)),
    label: 'Asked for the GPS trail',
  },
  {
    source: 'payments.cards',
    when: /\b(card|payment)s?\b/i,
    params: (t) => wrap('driver_id', first(DRIVER_ID, t)),
    label: 'Asked for payment card details',
  },
];

function wrap(key: string, v: string | null): Record<string, unknown> | null {
  return v === null ? null : { [key]: v };
}

export class FakeRouter implements Router {
  #rules: Rule[];

  constructor(rules: Rule[] = DEMO_RULES) {
    this.#rules = rules;
  }

  async route(input: RouteInput): Promise<RouteOutput> {
    const known = new Set(input.catalog.map((s) => s.source));
    const delivered = new Set(input.delivered);
    const requests: Proposal[] = [];
    for (const rule of this.#rules) {
      if (requests.length >= input.max) break;
      if (!known.has(rule.source) || delivered.has(rule.source) || !rule.when.test(input.text)) continue;
      const params = rule.params(input.text);
      if (!params) continue;
      const label = typeof rule.label === 'string' ? rule.label : rule.label(params);
      requests.push({ source: rule.source, params, label });
    }
    return { requests };
  }
}
