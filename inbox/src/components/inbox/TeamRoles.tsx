"use client";

import { useState } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, Badge, Button, Dialog, Input, KeyIcon } from "@/components/ds";
import type { PlatformRole, TeamMember } from "@/store/types";
import { rowBorder, segStyle } from "./styles";
import { chip, pill, ScopeEditor, scopeLabel } from "./ScopeEditor";
import { AddServiceAccountDialog, IssueKeyDialog, ServiceKeys } from "./ServiceAccounts";

export const TeamRoles = observer(function TeamRoles() {
  const store = useStore();
  const [editing, setEditing] = useState<{ memberId: string; role: PlatformRole } | null>(null);
  const [adding, setAdding] = useState<string | null>(null);
  const [inviting, setInviting] = useState(false);
  const [segment, setSegment] = useState<"people" | "service">("people");
  const [addingService, setAddingService] = useState(false);
  const [issuingFor, setIssuingFor] = useState<TeamMember | null>(null);
  const [linking, setLinking] = useState<TeamMember | null>(null);
  const canLink = store.can("team.manage");

  const people = store.team.filter((t) => t.kind !== "service");
  const services = store.team.filter((t) => t.kind === "service");
  const rows = segment === "service" ? services : people;

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
        {segment === "service" ? (
          <Button data-testid="add-service-account" onClick={() => setAddingService(true)}>
            Add service account
          </Button>
        ) : (
          <Button data-testid="invite-member" onClick={() => setInviting(true)}>
            Invite
          </Button>
        )}
      </div>

      <div style={{ padding: "11px 18px", borderBottom: rowBorder, display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ display: "inline-flex", gap: 2, padding: 3, borderRadius: 9, background: "var(--surface-sunken)", border: "1px solid var(--border-subtle)" }}>
          <button type="button" data-testid="team-seg-people" onClick={() => setSegment("people")} style={segStyle(segment === "people")}>
            People {people.length}
          </button>
          <button type="button" data-testid="team-seg-service" onClick={() => setSegment("service")} style={segStyle(segment === "service")}>
            Service accounts {services.length}
          </button>
        </div>
        {segment === "service" && (
          <span style={{ fontSize: 12.5, color: "var(--text-tertiary)" }}>Automated actors. Same roles, no login — they sign in with a key.</span>
        )}
      </div>

      <div style={{ padding: "8px 18px", display: "flex", gap: 12, borderBottom: rowBorder, fontSize: 11, fontWeight: 600, letterSpacing: ".03em", textTransform: "uppercase", color: "var(--text-tertiary)" }}>
        <div style={{ width: 220, flex: "none" }}>{segment === "service" ? "Service account" : "Member"}</div>
        <div style={{ flex: 1 }}>Role assignments</div>
      </div>

      {segment === "service" && services.length === 0 && (
        <div style={{ padding: "18px", fontSize: 13, color: "var(--text-tertiary)" }}>
          No service accounts yet. Add one for an assistant or another automated actor.
        </div>
      )}

      {rows.map((t) => (
        <div
          key={t.id}
          style={{ padding: "13px 18px", display: "flex", alignItems: "flex-start", gap: 12, borderBottom: rowBorder }}
          data-testid={t.kind === "service" ? "service-row" : "team-row"}
        >
          <div style={{ width: 220, flex: "none", display: "flex", alignItems: "center", gap: 10, minWidth: 0 }}>
            {t.kind === "service" ? (
              <div style={{ width: 36, height: 36, flex: "none", borderRadius: 9, background: "var(--surface-sunken)", border: "1px solid var(--border-default)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--text-tertiary)" }}>
                <KeyIcon size={17} />
              </div>
            ) : (
              <Avatar name={t.name} size={36} />
            )}
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 14, fontWeight: 600, display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
                <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{t.name}</span>
                {t.id === store.meId && (
                  <span style={{ fontSize: 11, fontWeight: 600, color: "var(--text-tertiary)" }}>· You</span>
                )}
                {t.kind === "service" && <Badge data-testid="service-badge">Service account</Badge>}
              </div>
              <div
                style={{
                  fontSize: t.kind === "service" ? 11.5 : 12,
                  fontFamily: t.kind === "service" ? "var(--font-mono)" : undefined,
                  color: "var(--text-tertiary)",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
              >
                {t.email}
              </div>
            </div>
          </div>

          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
              {t.assignments.map((a) => (
                <button
                  key={a.role}
                  data-testid={`assignment-${a.role}`}
                  onClick={() => setEditing({ memberId: t.id, role: a.role })}
                  style={chip(false)}
                >
                  <span style={{ fontWeight: 600 }}>{a.role}</span>
                  <span style={{ fontSize: 12.5, color: "var(--text-tertiary)" }}>
                    {scopeLabel(store.roleDef(a.role), a.scope, store.scopeOptionLabel)}
                  </span>
                </button>
              ))}
              <button data-testid="add-role" onClick={() => setAdding(t.id)} style={chip(true)}>
                + Add role
              </button>
            </div>
            {t.kind === "service" ? (
              <ServiceKeys member={t} onIssue={() => setIssuingFor(t)} />
            ) : (
              <IntegratorIdentity member={t} onEdit={canLink ? () => setLinking(t) : undefined} />
            )}
          </div>
        </div>
      ))}

      {addingService && <AddServiceAccountDialog onClose={() => setAddingService(false)} />}

      {issuingFor && <IssueKeyDialog member={issuingFor} onClose={() => setIssuingFor(null)} />}

      {linking && <LinkIdentityDialog member={linking} onClose={() => setLinking(null)} />}

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

/** Who the member is at the integrator, which embedded sign-in matches on. */
function IntegratorIdentity({ member, onEdit }: { member: TeamMember; onEdit?: () => void }) {
  return (
    <div style={{ marginTop: 8, display: "flex", alignItems: "center", gap: 8, fontSize: 12.5, color: "var(--text-tertiary)" }} data-testid="integrator-identity">
      <span>Integrator identity</span>
      {member.externalSub ? (
        <code data-testid="integrator-identity-value" style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-primary)", overflowWrap: "anywhere" }}>
          {member.externalSub}
        </code>
      ) : (
        <span data-testid="integrator-identity-value">Not linked</span>
      )}
      {onEdit && (
        <button type="button" data-testid="integrator-identity-edit" onClick={onEdit} style={chip(true)}>
          {member.externalSub ? "Edit" : "+ Link"}
        </button>
      )}
    </div>
  );
}

const LinkIdentityDialog = observer(function LinkIdentityDialog({ member, onClose }: { member: TeamMember; onClose: () => void }) {
  const store = useStore();
  const [sub, setSub] = useState(member.externalSub);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (value: string) => {
    setBusy(true);
    const err = await store.setExternalSub(member.id, value);
    setBusy(false);
    if (err) setError(err);
    else onClose();
  };

  return (
    <Dialog
      title={`Integrator identity — ${member.name}`}
      subtitle="The sub your validator returns for this person. A token for it signs in as them."
      onClose={onClose}
    >
      <Input
        label="Identity (sub)"
        data-testid="integrator-identity-input"
        placeholder="e.g. fleetline.7"
        value={sub}
        error={error ?? undefined}
        onChange={(e) => {
          setSub(e.target.value);
          setError(null);
        }}
      />
      <div style={{ marginTop: 18, display: "flex", alignItems: "center", gap: 10 }}>
        {member.externalSub && (
          <Button variant="secondary" data-testid="integrator-identity-unlink" onClick={() => void submit("")} disabled={busy}>
            Unlink
          </Button>
        )}
        <div style={{ flex: 1 }} />
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button data-testid="integrator-identity-save" onClick={() => void submit(sub)} disabled={busy || !sub.trim()}>
          Save
        </Button>
      </div>
    </Dialog>
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

// ScopeDialog edits one held assignment; every change is written as it is made.
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
  const assignment = store.assignmentOf(memberId, role);
  if (!assignment) return null;
  const name = store.memberName(memberId);

  return (
    <Dialog title={`${role} — ${name}`} subtitle={def?.description} onClose={onClose}>
      <ScopeEditor def={def} scope={assignment.scope} onChange={(next) => void store.setRoleScope(memberId, role, next)} />
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
        {/* Closing first would let a reload abort the last scope write. */}
        <Button
          onClick={async () => {
            await store.scopeSaved(memberId, role);
            onClose();
          }}
        >
          Done
        </Button>
      </div>
    </Dialog>
  );
});
