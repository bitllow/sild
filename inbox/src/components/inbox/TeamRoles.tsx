"use client";

import { useEffect, useState } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, Button, Dialog, Input, Switch } from "@/components/ds";
import type { PlatformRole, RoleDefinition, RoleDimension, RoleScope } from "@/store/types";
import { rowBorder } from "./styles";

const SCOPE_ALL = "all";

/** The chip's caption: what this assignment reaches, in the role's own words. */
function scopeLabel(def: RoleDefinition | undefined, scope: RoleScope): string {
  const dims = def?.dimensions ?? [];
  if (dims.length === 0) return "tenant-wide";
  const parts = dims.map((d) => dimensionLabel(d, scope)).filter(Boolean);
  return parts.length ? parts.join(" · ") : `no ${dims[0].label.toLowerCase()}`;
}

function dimensionLabel(d: RoleDimension, scope: RoleScope): string {
  if (d.kind === "toggle") return scope[d.key] ? d.label.toLowerCase() : "";
  const set = (scope[d.key] as string[] | undefined) ?? [];
  if (set.includes(SCOPE_ALL)) return `all ${d.label.toLowerCase()}`;
  if (set.length === 0) return `no ${d.label.toLowerCase()}`;
  return set.join(", ");
}

// A selectable pill: role picker in the invite, option chips in a scope.
const pill = (selected: boolean) => ({
  border: `1px solid ${selected ? "var(--brand)" : "var(--border-default)"}`,
  background: selected ? "var(--brand-subtle)" : "var(--white)",
  color: selected ? "var(--brand)" : "var(--text-secondary)",
  borderRadius: 999,
  padding: "5px 11px",
  cursor: "pointer",
  fontFamily: "var(--font-sans)",
  fontSize: 13,
  fontWeight: 600,
});

const chip = (dashed: boolean) => ({
  display: "flex",
  alignItems: "center",
  gap: 8,
  maxWidth: "100%",
  border: `1px ${dashed ? "dashed" : "solid"} var(--border-default)`,
  borderRadius: 999,
  background: dashed ? "transparent" : "var(--white)",
  padding: "5px 12px",
  cursor: "pointer",
  fontFamily: "var(--font-sans)",
  fontSize: 13,
  color: dashed ? "var(--text-tertiary)" : "var(--text-primary)",
});

export const TeamRoles = observer(function TeamRoles() {
  const store = useStore();
  const [editing, setEditing] = useState<{ memberId: string; role: PlatformRole } | null>(null);
  const [adding, setAdding] = useState<string | null>(null);
  const [inviting, setInviting] = useState(false);

  return (
    <>
      <div style={{ padding: "16px 18px", borderBottom: rowBorder, display: "flex", alignItems: "flex-start", gap: 12 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 15, fontWeight: 700 }}>Team</div>
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2, lineHeight: 1.5 }}>
            Access is a set of role assignments per member. A member may hold several. Each
            assignment carries the scope its role defines.
          </div>
          {store.teamError && (
            <div style={{ fontSize: 13, color: "var(--danger)", marginTop: 8 }} data-testid="team-error">
              {store.teamError}
            </div>
          )}
        </div>
        <Button data-testid="invite-member" onClick={() => setInviting(true)}>
          Invite
        </Button>
      </div>

      <div style={{ padding: "8px 18px", display: "flex", gap: 12, borderBottom: rowBorder, fontSize: 11, fontWeight: 600, letterSpacing: ".03em", textTransform: "uppercase", color: "var(--text-tertiary)" }}>
        <div style={{ width: 220, flex: "none" }}>Member</div>
        <div style={{ flex: 1 }}>Role assignments</div>
      </div>

      {store.team.map((t) => (
        <div key={t.id} style={{ padding: "13px 18px", display: "flex", alignItems: "flex-start", gap: 12, borderBottom: rowBorder }} data-testid="team-row">
          <div style={{ width: 220, flex: "none", display: "flex", alignItems: "center", gap: 10, minWidth: 0 }}>
            <Avatar name={t.name} size={36} />
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 14, fontWeight: 600 }}>
                {t.name}
                {t.id === store.meId && (
                  <span style={{ marginLeft: 6, fontSize: 11, fontWeight: 600, color: "var(--text-tertiary)" }}>· You</span>
                )}
              </div>
              <div style={{ fontSize: 12, color: "var(--text-tertiary)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {t.email}
              </div>
            </div>
          </div>

          <div style={{ flex: 1, minWidth: 0, display: "flex", flexWrap: "wrap", gap: 6 }}>
            {t.assignments.map((a) => (
              <button
                key={a.role}
                data-testid={`assignment-${a.role}`}
                onClick={() => setEditing({ memberId: t.id, role: a.role })}
                style={chip(false)}
              >
                <span style={{ fontWeight: 600 }}>{a.role}</span>
                <span style={{ fontSize: 12.5, color: "var(--text-tertiary)" }}>
                  {scopeLabel(store.roleDef(a.role), a.scope)}
                </span>
              </button>
            ))}
            <button data-testid="add-role" onClick={() => setAdding(t.id)} style={chip(true)}>
              + Add role
            </button>
          </div>
        </div>
      ))}

      {inviting && (
        <InviteDialog
          onClose={() => setInviting(false)}
          onInvited={(memberId, role) => {
            setInviting(false);
            setEditing({ memberId, role });
          }}
        />
      )}

      {editing && (
        <ScopeDialog
          memberId={editing.memberId}
          role={editing.role}
          onClose={() => setEditing(null)}
        />
      )}

      {adding && (
        <Dialog
          title="Add role"
          subtitle={`A second role adds capabilities to ${store.memberName(adding)}; it never takes any away.`}
          onClose={() => setAdding(null)}
        >
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {store.addableRoles(adding).map((d) => (
              <button
                key={d.role}
                data-testid={`add-role-${d.role}`}
                onClick={async () => {
                  const memberId = adding;
                  setAdding(null);
                  // Straight into the scope: a role granted with an empty scope
                  // reaches nothing, and nobody should have to discover that.
                  if (await store.addRole(memberId, d.role)) setEditing({ memberId, role: d.role });
                }}
                style={{ display: "flex", flexDirection: "column", gap: 3, textAlign: "left", width: "100%", padding: "11px 13px", border: "1px solid var(--border-default)", borderRadius: 10, background: "var(--white)", cursor: "pointer", fontFamily: "var(--font-sans)" }}
              >
                <span style={{ fontSize: 14, fontWeight: 600 }}>{d.label}</span>
                <span style={{ fontSize: 12.5, color: "var(--text-tertiary)", lineHeight: 1.5 }}>{d.description}</span>
              </button>
            ))}
            {store.addableRoles(adding).length === 0 && (
              <p style={{ fontSize: 13, color: "var(--text-tertiary)" }}>This member already holds every role.</p>
            )}
          </div>
        </Dialog>
      )}
    </>
  );
});

// InviteDialog adds someone to the tenant in one role. Its scope is chosen right
// afterwards, in the same dialog every other assignment uses.
const InviteDialog = observer(function InviteDialog({
  onClose,
  onInvited,
}: {
  onClose: () => void;
  onInvited: (memberId: string, role: PlatformRole) => void;
}) {
  const store = useStore();
  const [email, setEmail] = useState("");
  const [first, setFirst] = useState("");
  const [last, setLast] = useState("");
  const [role, setRole] = useState<PlatformRole>("agent");
  const [busy, setBusy] = useState(false);

  const invite = async () => {
    if (!email.trim() || busy) return;
    setBusy(true);
    const id = await store.inviteMember(email, first, last, role);
    setBusy(false);
    if (id) onInvited(id, role);
  };

  return (
    <Dialog
      title="Invite a member"
      subtitle="They join in one role; a second one can be added afterwards."
      onClose={onClose}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        <Input
          data-testid="invite-email"
          placeholder="name@company.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <div style={{ display: "flex", gap: 10 }}>
          <Input placeholder="First name" value={first} onChange={(e) => setFirst(e.target.value)} />
          <Input placeholder="Last name" value={last} onChange={(e) => setLast(e.target.value)} />
        </div>
        <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
          {store.roleDefs.map((d) => (
            <button
              key={d.role}
              data-testid={`invite-role-${d.role}`}
              onClick={() => setRole(d.role)}
              style={pill(role === d.role)}
            >
              {d.label}
            </button>
          ))}
        </div>
      </div>
      <div style={{ marginTop: 18, display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ flex: 1 }} />
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button data-testid="invite-send" onClick={invite} disabled={!email.trim() || busy}>
          Invite
        </Button>
      </div>
    </Dialog>
  );
});

// ScopeDialog renders the dimensions the role declares — a switch for a toggle,
// chips for a set.
const ScopeDialog = observer(function ScopeDialog({
  memberId,
  role,
  onClose,
}: {
  memberId: string;
  role: PlatformRole;
  onClose: () => void;
}) {
  const store = useStore();
  const def = store.roleDef(role);
  // Only a set dimension needs the tenant's projects and languages, so the
  // fetch waits until one is about to render. Idempotent.
  useEffect(() => {
    if ((def?.dimensions ?? []).some((d) => d.kind === "set")) void store.translations.load();
  }, [store, def]);
  const assignment = store.assignmentOf(memberId, role);
  if (!assignment) return null;
  const scope = assignment.scope;
  const name = store.memberName(memberId);

  const write = (next: RoleScope) => void store.setRoleScope(memberId, role, next);

  const toggleMember = (d: RoleDimension, value: string) => {
    const set = (scope[d.key] as string[] | undefined) ?? [];
    const next = set.includes(value) ? set.filter((v) => v !== value) : [...set, value];
    write({ ...scope, [d.key]: value === SCOPE_ALL && !set.includes(SCOPE_ALL) ? [SCOPE_ALL] : next });
  };

  return (
    <Dialog title={`${role} — ${name}`} subtitle={def?.description} onClose={onClose}>
      {(def?.dimensions ?? []).map((d) => (
        <div key={d.key} style={{ marginBottom: 18 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 13, fontWeight: 600 }}>{d.label}</div>
              <div style={{ fontSize: 12.5, color: "var(--text-tertiary)", marginTop: 2, lineHeight: 1.5 }}>{d.help}</div>
            </div>
            <div style={{ flex: 1 }} />
            {d.kind === "toggle" && (
              <Switch
                checked={!!scope[d.key]}
                onChange={(v) => write({ ...scope, [d.key]: v })}
              />
            )}
          </div>
          {d.kind === "set" && (
            <div style={{ display: "flex", gap: 6, marginTop: 9, flexWrap: "wrap" }}>
              {[SCOPE_ALL, ...store.scopeOptions(d.source)].map((option) => {
                const set = (scope[d.key] as string[] | undefined) ?? [];
                const on = set.includes(option);
                return (
                  <button
                    key={option}
                    data-testid={`scope-${d.key}-${option}`}
                    onClick={() => toggleMember(d, option)}
                    style={pill(on)}
                  >
                    {option}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      ))}
      {def && (def.dimensions ?? []).length === 0 && (
        <p style={{ fontSize: 13, color: "var(--text-tertiary)", lineHeight: 1.55 }}>
          This role is tenant-wide. It defines no scope of its own.
        </p>
      )}
      <div style={{ marginTop: 18, display: "flex", alignItems: "center", gap: 10 }}>
        <Button
          variant="secondary"
          data-testid="remove-role"
          onClick={async () => {
            if (await store.removeRole(memberId, role)) onClose();
          }}
        >
          Remove role
        </Button>
        <div style={{ flex: 1 }} />
        <Button onClick={onClose}>Done</Button>
      </div>
    </Dialog>
  );
});
