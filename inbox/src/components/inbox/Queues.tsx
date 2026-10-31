"use client";

import { useState } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Badge, Button, Dialog, Input, Switch } from "@/components/ds";
import type { ApiQueue } from "@/api/admin";
import { cardStyle as card, rowBorder } from "./styles";

const KEY_PATTERN = /^[a-z0-9_-]+$/;

export const Queues = observer(function Queues() {
  const store = useStore();
  // null: closed; "new": creating; otherwise the queue being edited.
  const [editing, setEditing] = useState<ApiQueue | "new" | null>(null);

  return (
    <div style={card} data-testid="queues-settings">
      <div style={{ padding: "16px 18px", borderBottom: rowBorder, display: "flex", alignItems: "flex-start", gap: 12 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 15, fontWeight: 700 }}>Queues</div>
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2, lineHeight: 1.5 }}>
            Where assignments wait to be claimed. An agent works the queues its role scope names;
            without one, support only.
          </div>
        </div>
        <Button
          data-testid="new-queue"
          onClick={() => {
            store.clearQueueError();
            setEditing("new");
          }}
        >
          New queue
        </Button>
      </div>

      {store.queues.map((q) => (
        <div key={q.key} data-testid="queue-row" style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 14, borderBottom: rowBorder }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span style={{ fontSize: 14, fontWeight: 600 }}>{q.label || q.key}</span>
              {q.built_in && <Badge>Built-in</Badge>}
            </div>
            <div style={{ marginTop: 3, fontSize: 12.5, color: "var(--text-tertiary)", display: "flex", gap: 8, flexWrap: "wrap" }}>
              <code style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{q.key}</code>
              <span>· {q.requeue_on_reply ? "requeues on reply" : "no requeue on reply"}</span>
              <span>· {q.webhooks ? "sends webhooks" : "no webhooks"}</span>
            </div>
          </div>
          {!q.built_in && (
            <Button
              size="sm"
              variant="secondary"
              data-testid={`edit-queue-${q.key}`}
              onClick={() => {
                store.clearQueueError();
                setEditing(q);
              }}
            >
              Edit
            </Button>
          )}
        </div>
      ))}

      {editing && <QueueDialog queue={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
    </div>
  );
});

const QueueDialog = observer(function QueueDialog({ queue, onClose }: { queue: ApiQueue | null; onClose: () => void }) {
  const store = useStore();
  const [key, setKey] = useState(queue?.key ?? "");
  const [label, setLabel] = useState(queue?.label ?? "");
  const [requeue, setRequeue] = useState(queue?.requeue_on_reply ?? false);
  const [webhooks, setWebhooks] = useState(queue?.webhooks ?? true);
  const [busy, setBusy] = useState(false);

  const k = key.trim();
  const taken = !queue && store.queues.some((q) => q.key === k);
  const keyError = !k || KEY_PATTERN.test(k) ? (taken ? "A queue with this key exists." : undefined) : "Lowercase letters, digits, - or _ only.";

  const save = async () => {
    if (!k || keyError || busy) return;
    setBusy(true);
    const ok = await store.saveQueue(k, { label: label.trim(), requeue_on_reply: requeue, webhooks });
    setBusy(false);
    if (ok) onClose();
  };

  return (
    <Dialog
      title={queue ? `Edit queue — ${queue.label || queue.key}` : "New queue"}
      subtitle={queue ? undefined : "The key is permanent: role scopes and the API refer to the queue by it."}
      onClose={onClose}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        <Input
          label="Key"
          data-testid="queue-key"
          placeholder="triage"
          value={key}
          disabled={!!queue}
          error={keyError}
          onChange={(e) => setKey(e.target.value)}
        />
        <Input
          label="Label"
          data-testid="queue-label"
          placeholder="Tech support"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
        />
      </div>
      <QueueToggle
        title="Requeue on reply"
        desc="A client reply on an open conversation with no open assignment queues it again."
        checked={requeue}
        onChange={setRequeue}
        testId="queue-requeue"
      />
      <QueueToggle
        title="Send webhooks"
        desc="Deliver this queue's events to the workspace's webhook endpoints."
        checked={webhooks}
        onChange={setWebhooks}
        testId="queue-webhooks"
      />
      {store.queueError && (
        <div style={{ fontSize: 13, color: "var(--danger)", marginTop: 12 }} data-testid="queue-error">
          {store.queueError}
        </div>
      )}
      <div style={{ marginTop: 18, display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ flex: 1 }} />
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button data-testid="queue-save" onClick={save} disabled={!k || !!keyError || busy}>
          {queue ? "Save" : "Create"}
        </Button>
      </div>
    </Dialog>
  );
});

function QueueToggle(props: { title: string; desc: string; checked: boolean; onChange: (v: boolean) => void; testId: string }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 14, marginTop: 16 }}>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 13, fontWeight: 600 }}>{props.title}</div>
        <div style={{ fontSize: 12.5, color: "var(--text-tertiary)", marginTop: 2, lineHeight: 1.5 }}>{props.desc}</div>
      </div>
      <Switch checked={props.checked} onChange={props.onChange} data-testid={props.testId} />
    </div>
  );
}
