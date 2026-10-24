"use client";

import { useState } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Button, Dialog, Input } from "@/components/ds";
import type { PlatformRole, RoleScope, TeamMember } from "@/store/types";
import { chip, pill, ScopeEditor } from "./ScopeEditor";
import { KeyRow } from "./KeyDialog";

/** The keys a service account signs in with, listed under its role chips. */
export const ServiceKeys = observer(function ServiceKeys({
  member,
  onIssue,
}: {
  member: TeamMember;
  onIssue: () => void;
}) {
  const store = useStore();
  const keys = store.keysOf(member.id);
  return (
    <div style={{ marginTop: 10, display: "flex", flexDirection: "column", gap: 6 }} data-testid="service-keys">
      {keys.map((k) => (
        <KeyRow key={k.id} k={k} compact />
      ))}
      <div>
        <button type="button" data-testid="issue-key" onClick={onIssue} style={chip(true)}>
          + Issue key
        </button>
      </div>
    </div>
  );
});

function KeyLabelStep({
  label,
  setLabel,
  busy,
  onSkip,
  onIssue,
  skipText,
}: {
  label: string;
  setLabel: (v: string) => void;
  busy: boolean;
  onSkip: () => void;
  onIssue: () => void;
  skipText: string;
}) {
  return (
    <>
      <Input
        label="Key label"
        data-testid="service-key-label"
        placeholder="e.g. Production runner"
        value={label}
        onChange={(e) => setLabel(e.target.value)}
      />
      <div style={{ marginTop: 18, display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ flex: 1 }} />
        <Button variant="secondary" onClick={onSkip}>
          {skipText}
        </Button>
        <Button data-testid="service-key-issue" onClick={onIssue} disabled={busy}>
          Issue key
        </Button>
      </div>
    </>
  );
}

// IssueKeyDialog names a new key; the store then reveals it once in KeyDialog.
export const IssueKeyDialog = observer(function IssueKeyDialog({
  member,
  onClose,
}: {
  member: TeamMember;
  onClose: () => void;
}) {
  const store = useStore();
  const [label, setLabel] = useState("");
  const [busy, setBusy] = useState(false);
  const issue = async () => {
    setBusy(true);
    const ok = await store.issueServiceKey(member.id, label);
    setBusy(false);
    if (ok) onClose();
  };
  return (
    <Dialog
      title={`Issue a key — ${member.name}`}
      subtitle="The key signs in as this service account, with its roles. It is shown once."
      onClose={onClose}
    >
      {store.teamError && <div style={{ fontSize: 13, color: "var(--danger)", marginBottom: 10 }}>{store.teamError}</div>}
      <KeyLabelStep label={label} setLabel={setLabel} busy={busy} onSkip={onClose} onIssue={issue} skipText="Cancel" />
    </Dialog>
  );
});

// AddServiceAccountDialog creates the account in one role and scope, then offers
// its first key: without one the account cannot act.
export const AddServiceAccountDialog = observer(function AddServiceAccountDialog({
  onClose,
}: {
  onClose: () => void;
}) {
  const store = useStore();
  const [name, setName] = useState("");
  const [display, setDisplay] = useState("");
  const [role, setRole] = useState<PlatformRole>("agent");
  const [scope, setScope] = useState<RoleScope>({});
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  const [createdId, setCreatedId] = useState<string | null>(null);
  const [keyLabel, setKeyLabel] = useState("");

  const create = async () => {
    if (!name.trim() || busy) return;
    setBusy(true);
    setTried(true);
    const id = await store.createServiceAccount(name, display || name, role, scope);
    setBusy(false);
    if (id) setCreatedId(id);
  };

  const issue = async () => {
    if (!createdId) return;
    setBusy(true);
    const ok = await store.issueServiceKey(createdId, keyLabel);
    setBusy(false);
    if (ok) onClose();
  };

  if (createdId) {
    return (
      <Dialog
        title="Service account created"
        subtitle="It has no password. Issue a key for whatever runs it — the key is shown once."
        onClose={onClose}
      >
        {store.teamError && <div style={{ fontSize: 13, color: "var(--danger)", marginBottom: 10 }}>{store.teamError}</div>}
        <KeyLabelStep label={keyLabel} setLabel={setKeyLabel} busy={busy} onSkip={onClose} onIssue={issue} skipText="Later" />
      </Dialog>
    );
  }

  return (
    <Dialog
      title="Add a service account"
      subtitle="An automated actor, such as an assistant. It holds roles like a member and signs in with a key."
      onClose={onClose}
      width={540}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        <Input
          label="Name"
          data-testid="service-name"
          placeholder="triage-assistant"
          hint="Unique in this workspace. Keys and logs refer to it by this."
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Input
          label="Display name"
          data-testid="service-display-name"
          placeholder="Triage assistant"
          hint="Shown on its messages."
          value={display}
          onChange={(e) => setDisplay(e.target.value)}
        />
        <div style={{ fontSize: 13, fontWeight: 600, marginTop: 6 }}>Role</div>
        <div style={{ display: "flex", gap: 6, flexWrap: "wrap", marginBottom: 8 }}>
          {store.roleDefs.map((d) => (
            <button
              key={d.role}
              type="button"
              data-testid={`service-role-${d.role}`}
              onClick={() => {
                setRole(d.role);
                setScope({});
              }}
              style={pill(role === d.role)}
            >
              {d.label}
            </button>
          ))}
        </div>
        <ScopeEditor def={store.roleDef(role)} scope={scope} onChange={setScope} />
      </div>
      {tried && store.teamError && (
        <div style={{ fontSize: 13, color: "var(--danger)", marginTop: 4 }} data-testid="service-error">
          {store.teamError}
        </div>
      )}
      <div style={{ marginTop: 18, display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ flex: 1 }} />
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button data-testid="service-create" onClick={create} disabled={!name.trim() || busy}>
          Create
        </Button>
      </div>
    </Dialog>
  );
});
