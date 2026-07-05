"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, InboxIcon, SettingsIcon } from "@/components/ds";
import { navStyle } from "./styles";
import { ConversationList } from "./ConversationList";
import { ConversationView } from "./ConversationView";
import { MemberPanel } from "./MemberPanel";
import { Settings } from "./Settings";
import { KeyDialog } from "./KeyDialog";

export const Shell = observer(function Shell() {
  const store = useStore();
  const isList = store.inboxView === "inbox";

  return (
    <div style={{ height: "100%", display: "flex" }}>
      {/* Nav rail */}
      <div
        style={{
          width: 64,
          flex: "none",
          background: "var(--surface-card)",
          borderRight: "1px solid var(--border-default)",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          padding: "14px 0",
          gap: 6,
        }}
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src="/assets/sild-mark-tile.svg" width={34} alt="Sild" style={{ borderRadius: 10, marginBottom: 8 }} />
        <div style={{ position: "relative" }}>
          <button onClick={store.goInbox} aria-label="Inbox" style={navStyle(isList)}>
            <InboxIcon size={22} />
          </button>
          {store.attentionCount > 0 && (
            <span
              data-testid="nav-attention"
              style={{
                position: "absolute",
                top: -3,
                right: -3,
                minWidth: 18,
                height: 18,
                padding: "0 5px",
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "center",
                background: "var(--coral-500)",
                color: "#fff",
                fontFamily: "var(--font-sans)",
                fontSize: 11,
                fontWeight: 700,
                lineHeight: 1,
                borderRadius: 9,
                border: "2px solid var(--surface-card)",
                pointerEvents: "none",
              }}
            >
              {store.attentionCount}
            </span>
          )}
        </div>
        <button onClick={store.goSettings} aria-label="Settings" style={navStyle(!isList)}>
          <SettingsIcon size={22} />
        </button>
        <div style={{ flex: 1 }} />
        <button
          onClick={store.logout}
          aria-label="Sign out"
          style={{ border: 0, background: "transparent", cursor: "pointer", padding: 0, borderRadius: "50%" }}
        >
          <Avatar name="Agent" size={36} />
        </button>
      </div>

      {/* Stage */}
      {isList ? (
        <div style={{ flex: 1, minWidth: 0, display: "flex" }}>
          <ConversationList />
          <ConversationView />
          {store.panelOpen && <MemberPanel />}
        </div>
      ) : (
        <Settings />
      )}

      {store.keyDialog && <KeyDialog />}
    </div>
  );
});
