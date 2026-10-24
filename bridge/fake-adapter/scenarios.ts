// The design's four scenarios (SildTechSupport.dc.html SEED) as adapter sources.
import type { Final, InfoRequest, Instructions, Outcome, Source } from '../src/adapter/kit/index.ts';

export const INSTRUCTIONS: Instructions = {
  version: '2026-10-01',
  text: `Fleetline is the admin app taxi companies use to run their fleet. Dispatchers assign trips to drivers; the office manages drivers, vehicles, invoices and the link to the company's accounting system.

Terms
- Driver: takes trips in the Fleetline driver app. Ids look like drv_2207.
- Dispatcher: office staff who assign trips and watch the live map.
- Trip: one ride, ids like TR-48213. "Pending" means assigned but not yet accepted by the driver.
- Vehicle: known by its 17-character VIN. A vehicle must pass VIN validation before a driver can use it.
- ERP webhook: sends completed trips and invoices to the company's accounting (ERP) system.
- Invoice: issued once a month from completed trips.

How to answer
- Answer in the language the requester writes in.
- Use plain words: "the driver's phone is not getting trip alerts", not "the push token is stale". Use a technical term only if the requester did.

Where to send people
- Billing disputes and invoice corrections go to billing@fleetline.example. You cannot change an invoice.
- Never promise a refund, credit or compensation; only the billing team decides that.`,
};

const obj = (properties: Record<string, unknown>, required: string[] = Object.keys(properties)) => ({
  type: 'object',
  properties,
  required,
  additionalProperties: false,
});
const DRIVER_ID = { type: 'string', pattern: '^drv_[0-9]+$', description: 'Driver id, e.g. drv_2207' };
const VIN = { type: 'string', pattern: '^[A-HJ-NPR-Z0-9]{17}$', description: '17-character vehicle identification number' };
const MONTH = { type: 'string', pattern: '^20[0-9]{2}-(0[1-9]|1[0-2])$', description: 'Month as YYYY-MM' };

export const CATALOG: Source[] = [
  { source: 'driver.lookup', description: 'Find a driver by name, phone number or email. Returns id, status and shift.', params: obj({ query: { type: 'string', description: 'Name, phone or email' } }), max_wait_seconds: 10 },
  { source: 'driver.devices', description: "A driver's registered phones: app version and push-notification token state.", params: obj({ driver_id: DRIVER_ID }), max_wait_seconds: 20 },
  { source: 'dispatch.pending', description: 'Trips assigned to a driver that have not been accepted yet, with delivery attempts.', params: obj({ driver_id: DRIVER_ID }), max_wait_seconds: 20 },
  { source: 'dispatch.available', description: 'Drivers available now near the pickup of a trip.', params: obj({ trip_id: { type: 'string', pattern: '^TR-[0-9]+$' }, radius_km: { type: 'number', minimum: 1, maximum: 50 } }, ['trip_id']), max_wait_seconds: 10 },
  { source: 'webhook.deliveries', description: 'A webhook endpoint and its latest delivery attempts. Find it by name (e.g. ERP) or id.', params: obj({ webhook: { type: 'string' } }), max_wait_seconds: 10 },
  { source: 'webhook.audit', description: 'Recent configuration changes to a webhook (secret rotation, URL edits, pauses).', params: obj({ webhook_id: { type: 'string', pattern: '^wh_[a-z0-9_]+$' } }), max_wait_seconds: 20 },
  { source: 'vehicle.validate', description: 'Run the platform VIN validation on a VIN and return every rule that fails.', params: obj({ vin: VIN }), max_wait_seconds: 10 },
  { source: 'vehicle.registry', description: 'Look a VIN up in the national vehicle registry.', params: obj({ vin: VIN }), max_wait_seconds: 20 },
  { source: 'invoices.export', description: 'An invoice export for a month: order count and grouping.', params: obj({ month: MONTH }), max_wait_seconds: 10 },
  { source: 'orders.completed', description: 'Orders completed in a month, with their invoice dates (CSV).', params: obj({ month: MONTH }), max_wait_seconds: 20 },
  { source: 'activity.errors', description: 'Error log of a service over the last N days.', params: obj({ service: { type: 'string', enum: ['driver-app', 'dispatch', 'webhooks', 'billing'] }, days: { type: 'integer', minimum: 1, maximum: 90 } }), max_wait_seconds: 3 },
  { source: 'service.status', description: 'Current status of platform components.', params: obj({}), max_wait_seconds: 10 },
  { source: 'payments.cards', description: "A driver's payout cards.", params: obj({ driver_id: DRIVER_ID }), max_wait_seconds: 10 },
  { source: 'telematics.gps', description: 'GPS trail of a vehicle for the last hour.', params: obj({ vehicle: { type: 'string' } }), max_wait_seconds: 15 },
];

const DRIVERS = [
  { id: 'drv_2207', name: 'Andres Kuusk', phone: '+372 5123 4567', email: 'andres.kuusk@example.ee', status: 'active', shift: '07:00–15:00', fleet: 'Tallinn Central' },
  { id: 'drv_2311', name: 'Mari Lepp', phone: '+372 5550 1122', email: 'mari.lepp@example.ee', status: 'active', shift: '06:00–14:00', fleet: 'Tallinn Central' },
  { id: 'drv_1984', name: 'Toomas Rand', phone: '+372 5888 3141', email: 'toomas.rand@example.ee', status: 'active', shift: '09:00–17:00', fleet: 'Tallinn Central' },
];

const today = () => new Date().toISOString().slice(0, 10);
const at = (hhmm: string) => `${today()}T${hhmm}:00+03:00`;
const json = (name: string, v: unknown) => ({ name, mime: 'application/json', data: JSON.stringify(v, null, 2) });
const deliver = (summary: string, ...files: ReturnType<typeof json>[]): Final => ({ type: 'deliver', summary, files });
const later = (ms: number, f: () => Final): Outcome => ({ type: 'async', result: new Promise((r) => setTimeout(() => r(f()), ms * delayScale())) });

// FAKE_DELAY_SCALE=0 answers async sources at once (tests).
function delayScale(): number {
  const v = Number(process.env.FAKE_DELAY_SCALE ?? '1');
  return Number.isFinite(v) && v >= 0 ? v : 1;
}

const digits = (p: string) => p.replace(/\D/g, '');

function findDriver(query: string) {
  const q = query.trim().toLowerCase();
  return DRIVERS.filter(
    (d) => d.name.toLowerCase().includes(q) || d.email === q || (digits(q).length >= 7 && digits(d.phone).endsWith(digits(q))),
  );
}

const VIN_VALUES: Record<string, number> = {
  A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, J: 1, K: 2, L: 3, M: 4, N: 5, P: 7, R: 9, S: 2, T: 3, U: 4, V: 5, W: 6, X: 7, Y: 8, Z: 9,
};
const VIN_WEIGHTS = [8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2];

export function vinCheckDigit(vin: string): string {
  const sum = [...vin].reduce((n, ch, i) => n + (/\d/.test(ch) ? Number(ch) : (VIN_VALUES[ch] ?? 0)) * VIN_WEIGHTS[i]!, 0);
  const r = sum % 11;
  return r === 10 ? 'X' : String(r);
}

const MONTH_NAMES = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

function monthOrders(month: string) {
  const [y, m] = month.split('-').map(Number) as [number, number];
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate();
  const rows: { order: string; completed: string; invoiced: string; amount_eur: string }[] = [];
  for (let i = 0; i < 219; i++) {
    const day = i < 212 ? 1 + (i % (last - 1)) : last;
    const completed = `${month}-${String(day).padStart(2, '0')}`;
    const invoiced = day === last ? `${m === 12 ? y + 1 : y}-${String((m % 12) + 1).padStart(2, '0')}-01` : completed;
    rows.push({ order: `ORD-${y}${String(m).padStart(2, '0')}-${String(i + 1).padStart(4, '0')}`, completed, invoiced, amount_eur: (8 + ((i * 37) % 4200) / 100).toFixed(2) });
  }
  return { rows, lastDay: last, name: MONTH_NAMES[m - 1]! };
}

// Params are already validated against the catalog schema by the kit.
type P = Record<string, any>;

const HANDLERS: Record<string, (p: P, req: InfoRequest) => Outcome> = {
  'driver.lookup': (p) => {
    const found = findDriver(p.query);
    const d = found[0];
    if (!d) return deliver(`No driver matches "${p.query.slice(0, 60)}"`, json('drivers.json', { query: p.query, matches: [] }));
    return deliver(`${d.id} · ${d.status} · shift ${d.shift}`, json('drivers.json', { query: p.query, matches: found }));
  },

  'driver.devices': (p) =>
    later(1500, () => {
      const expired = p.driver_id === 'drv_2207';
      const device = {
        platform: 'android',
        model: 'Samsung Galaxy A54',
        app_version: expired ? '5.12.0' : '5.11.3',
        app_updated_at: expired ? at('07:41') : at('06:02'),
        push_token: expired
          ? { status: 'expired', expired_at: at('07:42'), last_success: at('07:39'), provider_error: 'UNREGISTERED' }
          : { status: 'valid', last_success: at('10:12') },
      };
      return deliver(
        expired ? 'Notification token expired 07:42, after app update 5.12.0' : 'Notification token valid, app 5.11.3',
        json('devices.json', { driver_id: p.driver_id, devices: [device] }),
      );
    }),

  'dispatch.pending': (p) =>
    later(2000, () => {
      const trips =
        p.driver_id === 'drv_2207'
          ? [
              { trip: 'TR-88412', pickup_at: at('11:30'), pickup: 'Narva mnt 7, Tallinn', assigned_at: at('09:58') },
              { trip: 'TR-88437', pickup_at: at('12:15'), pickup: 'Pärnu mnt 22, Tallinn', assigned_at: at('10:06') },
              { trip: 'TR-88451', pickup_at: at('13:00'), pickup: 'Tartu mnt 80, Tallinn', assigned_at: at('10:21') },
            ].map((t) => ({ ...t, state: 'pending', delivery_attempts: [{ at: t.assigned_at, result: 'failed', reason: 'push_token_expired' }] }))
          : [];
      const summary = trips.length ? `${trips.length} trips pending, delivery failed` : 'No trips pending';
      return deliver(summary, json('pending-trips.json', { driver_id: p.driver_id, trips }));
    }),

  'dispatch.available': (p) =>
    deliver('2 available within 5 km', json('available-drivers.json', {
      trip: p.trip_id,
      radius_km: p.radius_km ?? 5,
      drivers: [
        { id: 'drv_2311', name: 'Mari Lepp', distance_km: 2.1, eta_min: 6 },
        { id: 'drv_1984', name: 'Toomas Rand', distance_km: 3.4, eta_min: 9 },
      ],
    })),

  'webhook.deliveries': (p) => {
    if (!/erp|wh_erp/i.test(p.webhook)) return deliver(`No webhook matches "${p.webhook.slice(0, 60)}"`, json('webhook.json', { query: p.webhook, matches: [] }));
    const attempts = ['10:38', '10:31', '10:24', '10:17', '10:10'].map((t, i) => ({
      id: `whd_${9041 - i}`, at: at(t), event: 'order.completed', status: 401, response: 'Unauthorized: invalid signature', duration_ms: 140 + i * 7,
    }));
    return deliver('41 failed deliveries since 08:02; erp.northwind.ee answers 401 Unauthorized', json('webhook.json', {
      webhook: { id: 'wh_erp', name: 'ERP sync', url: 'https://erp.northwind.ee/hooks/orders', events: ['order.completed', 'order.cancelled'], state: 'active' },
      failing_since: at('08:02'),
      failed_count: 41,
      last_success: at('07:57'),
      attempts,
    }));
  },

  'webhook.audit': (p) =>
    later(2000, () =>
      p.webhook_id === 'wh_erp'
        ? deliver('Signing secret rotated 07:58 by Eva Kask', json('audit.json', {
            resource: p.webhook_id,
            entries: [
              { at: at('07:58'), actor: 'Eva Kask', action: 'webhook.secret.rotated', detail: 'previous secret revoked immediately' },
              { at: '2026-08-14T13:20:00+03:00', actor: 'Eva Kask', action: 'webhook.events.updated', detail: 'added order.cancelled' },
            ],
          }))
        : deliver('No changes in the last 30 days', json('audit.json', { resource: p.webhook_id, entries: [] })),
    ),

  'vehicle.validate': (p) => {
    const expected = vinCheckDigit(p.vin);
    const actual = p.vin[8]!;
    const ok = expected === actual;
    return deliver(
      ok ? 'VIN passes validation' : `Check digit does not match (position 9 is '${actual}', expected '${expected}')`,
      json('validation.json', {
        vin: p.vin,
        status: ok ? 200 : 422,
        rules: [
          { rule: 'length_17', ok: true },
          { rule: 'allowed_characters', ok: true },
          { rule: 'iso3779_check_digit', ok, ...(ok ? {} : { position: 9, actual, expected }) },
        ],
        wmi: { code: p.vin.slice(0, 3), manufacturer: p.vin.startsWith('WV1') ? 'Volkswagen Commercial Vehicles' : 'unknown', region: /^[S-Z]/.test(p.vin) ? 'Europe' : 'other' },
      }),
    );
  },

  'vehicle.registry': (p) =>
    later(3000, () =>
      p.vin.startsWith('WV1')
        ? deliver('Found: Volkswagen Transporter T6.1 (2023), registered in Estonia', json('registry.json', {
            vin: p.vin, found: true, make: 'Volkswagen', model: 'Transporter T6.1', year: 2023, registered_in: 'EE', first_registration: '2023-04-18',
            note: 'European manufacturers are not required to use the ISO 3779 check digit at position 9.',
          }))
        : deliver('VIN not found in the registry', json('registry.json', { vin: p.vin, found: false })),
    ),

  'invoices.export': (p) => {
    const { rows, name } = monthOrders(p.month);
    const invoiced = rows.filter((r) => r.invoiced.startsWith(p.month));
    return deliver(`${invoiced.length} orders, grouped by invoice date`, json('export.json', {
      id: `exp_${p.month.replace('-', '_')}`, month: p.month, title: `${name} invoice export`, grouped_by: 'invoice_date', order_count: invoiced.length,
      total_eur: invoiced.reduce((n, r) => n + Number(r.amount_eur), 0).toFixed(2),
    }));
  },

  'orders.completed': (p) =>
    later(2500, () => {
      const { rows, lastDay, name } = monthOrders(p.month);
      const moved = rows.filter((r) => !r.invoiced.startsWith(p.month));
      const csv = ['order,completed,invoiced,amount_eur', ...rows.map((r) => `${r.order},${r.completed},${r.invoiced},${r.amount_eur}`)].join('\n');
      return {
        type: 'deliver',
        summary: `${rows.length} orders completed in ${name}; ${moved.length} completed ${lastDay} ${name.slice(0, 3)} were invoiced the next month`,
        files: [
          { name: 'orders.csv', mime: 'text/csv', data: csv, byUrl: true },
          { name: 'invoiced-next-month.json', mime: 'application/json', data: JSON.stringify(moved, null, 2) },
        ],
      };
    }),

  'activity.errors': (p) =>
    // A long range never finishes; the kit reports it expired after max_wait.
    p.days > 30 ? { type: 'async', result: new Promise<Final>(() => {}) } : later(1000, () => deliver(
      `12 errors in ${p.service} over ${p.days} day(s); top: PushTokenInvalid (9)`,
      json('errors.json', {
        service: p.service, days: p.days,
        top: [{ error: 'PushTokenInvalid', count: 9, first: at('07:42') }, { error: 'GpsFixTimeout', count: 3, first: at('08:15') }],
      }),
    )),

  'service.status': () =>
    deliver('All systems operational', json('status.json', {
      checked_at: new Date().toISOString(),
      components: ['api', 'dispatch', 'driver-push', 'webhooks', 'billing'].map((c) => ({ component: c, status: 'operational' })),
    })),

  'payments.cards': () => ({ type: 'decline', reason: 'Payout card data is not available to support staff' }),

  // Accepted and never answered.
  'telematics.gps': () => ({ type: 'accepted' }),
};

export function handle(req: InfoRequest): Outcome {
  const h = HANDLERS[req.source];
  if (!h) return { type: 'decline', reason: 'unknown source' };
  return h(req.params as P, req);
}
