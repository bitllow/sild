// Checks a validator endpoint against Sild's contract (docs/embedded-sign-in.md).
//
//   VALIDATOR_TOKEN=… node check-validator.ts <validator-url> <staff-token> [<refused-token>]
//
// staff-token must belong to someone allowed in; refused-token, when given, to
// someone the validator turns away (e.g. a driver). Exits 1 on any failure.

const [url, staffToken, refusedToken] = process.argv.slice(2);
const apiToken = process.env.VALIDATOR_TOKEN ?? "";
if (!url || !staffToken || !apiToken) {
  console.error("usage: VALIDATOR_TOKEN=… node check-validator.ts <validator-url> <staff-token> [<refused-token>]");
  process.exit(2);
}

const REASON = /^[a-z0-9_.-]{1,64}$/;
let failures = 0;

function check(name: string, ok: boolean, detail: unknown) {
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${ok ? "" : ` — ${JSON.stringify(detail)}`}`);
  if (!ok) failures++;
}

async function call(token: string, bearer = apiToken): Promise<{ status: number; body: any }> {
  const res = await fetch(url, {
    method: "POST",
    redirect: "manual",
    signal: AbortSignal.timeout(5000),
    headers: { Authorization: `Bearer ${bearer}`, "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
  });
  const text = await res.text();
  try {
    return { status: res.status, body: JSON.parse(text) };
  } catch {
    return { status: res.status, body: text };
  }
}

const isInactive = (r: { status: number; body: any }) =>
  r.status === 200 && r.body?.active === false && REASON.test(r.body.reason ?? "inactive");

const staff = await call(staffToken);
check(
  "staff token → 200 active with sub",
  staff.status === 200 && staff.body?.active === true && typeof staff.body.sub === "string" && staff.body.sub !== "" &&
    staff.body.sub.length <= 255 && (staff.body.exp === undefined || Number.isInteger(staff.body.exp)),
  staff,
);
check("wrong API token → non-2xx", (await call(staffToken, `${apiToken}-wrong`)).status >= 300, "answered 2xx");
check("garbage token → 200 inactive with a reason", isInactive(await call("not-a-token")), "see contract");
if (refusedToken) {
  const refused = await call(refusedToken);
  check("refused token → 200 inactive with a reason", isInactive(refused), refused);
}

console.log(failures ? `\n${failures} check(s) failed` : "\nvalidator conforms");
process.exit(failures ? 1 : 0);
