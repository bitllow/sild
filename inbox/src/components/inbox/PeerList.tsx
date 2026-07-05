"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { AvatarStack, SearchIcon, PeopleIcon, CloseIcon } from "@/components/ds";

export const PeerList = observer(function PeerList() {
  const store = useStore();
  const peer = store.peer;
  const rows = peer.filtered;

  return (
    <div
      style={{
        width: 360,
        flex: "none",
        borderRight: "1px solid var(--border-default)",
        background: "var(--surface-card)",
        display: "flex",
        flexDirection: "column",
        minHeight: 0,
      }}
    >
      {/* Header */}
      <div style={{ padding: "18px 16px 10px", flex: "none" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700, letterSpacing: "-.01em" }}>Peer conversations</h2>
          <span
            style={{
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              minWidth: 20,
              height: 20,
              padding: "0 6px",
              borderRadius: 999,
              background: "var(--surface-sunken)",
              fontSize: 11,
              fontWeight: 700,
              color: "var(--text-tertiary)",
            }}
          >
            {peer.conversations.length}
          </span>
        </div>
        <p style={{ margin: "6px 0 0", fontSize: 12, color: "var(--text-tertiary)", lineHeight: 1.5 }}>
          Direct chats between parties — no support agent required. No assignment; you're here to observe, or step in
          if needed.
        </p>
      </div>

      {/* Search with autocomplete */}
      <div style={{ padding: "4px 16px 10px", flex: "none", position: "relative" }}>
        <div style={{ position: "relative" }}>
          <span style={{ position: "absolute", left: 10, top: "50%", transform: "translateY(-50%)", color: "var(--text-tertiary)", display: "flex" }}>
            <SearchIcon size={16} />
          </span>
          <input
            data-testid="peer-search"
            value={peer.query}
            placeholder="Search participants, trip, or role…"
            onFocus={peer.openSearch}
            onChange={(e) => peer.setQuery(e.target.value)}
            style={{
              width: "100%",
              height: 38,
              padding: "0 12px 0 32px",
              border: "1px solid var(--border-default)",
              borderRadius: 10,
              background: "var(--surface-card)",
              font: "inherit",
              fontSize: 13,
              color: "var(--text-primary)",
              outline: "none",
            }}
          />
        </div>

        {peer.searchOpen && (peer.roleSuggestions.length > 0 || peer.convSuggestions.length > 0) && (
          <>
            {/* scrim closes the dropdown */}
            <div onClick={peer.closeSearch} style={{ position: "fixed", inset: 0, zIndex: 20 }} />
            <div
              style={{
                position: "absolute",
                left: 16,
                right: 16,
                top: 48,
                zIndex: 21,
                background: "var(--surface-card)",
                border: "1px solid var(--border-default)",
                borderRadius: 12,
                boxShadow: "0 12px 32px rgba(20,24,31,.14)",
                padding: 6,
                maxHeight: 340,
                overflowY: "auto",
              }}
            >
              <div style={{ fontSize: 11, fontWeight: 600, letterSpacing: ".04em", textTransform: "uppercase", color: "var(--text-tertiary)", padding: "6px 10px 4px" }}>
                Filter by role
              </div>
              {peer.roleSuggestions.map((sg) => (
                <button
                  key={sg.role}
                  data-testid="peer-role-suggestion"
                  onClick={() => peer.pickRole(sg.role)}
                  style={{ display: "flex", alignItems: "center", gap: 8, width: "100%", textAlign: "left", padding: "8px 10px", border: 0, borderRadius: 8, background: "transparent", cursor: "pointer", color: "var(--text-primary)" }}
                >
                  <span style={{ color: "var(--text-tertiary)", display: "flex" }}>
                    <PeopleIcon size={16} />
                  </span>
                  <span style={{ flex: 1, fontWeight: 600, fontSize: 13 }}>{sg.role}</span>
                  <span style={{ fontSize: 12, color: "var(--text-tertiary)" }}>{sg.count}</span>
                </button>
              ))}
              {peer.convSuggestions.length > 0 && (
                <>
                  <div style={{ fontSize: 11, fontWeight: 600, letterSpacing: ".04em", textTransform: "uppercase", color: "var(--text-tertiary)", padding: "8px 10px 4px", borderTop: "1px solid var(--border-subtle)", marginTop: 4 }}>
                    Conversations
                  </div>
                  {peer.convSuggestions.map((sg) => (
                    <button
                      key={sg.id}
                      onClick={() => {
                        peer.closeSearch();
                        peer.setQuery("");
                        void peer.setActive(sg.id);
                      }}
                      style={{ display: "flex", alignItems: "center", gap: 8, width: "100%", textAlign: "left", padding: "8px 10px", border: 0, borderRadius: 8, background: "transparent", cursor: "pointer" }}
                    >
                      <span style={{ flex: 1, fontSize: 13, fontWeight: 600, color: "var(--text-primary)" }}>{sg.title}</span>
                      <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-tertiary)" }}>{sg.reference}</span>
                    </button>
                  ))}
                </>
              )}
            </div>
          </>
        )}
      </div>

      {/* Active role-filter pill */}
      {peer.roleFilter && (
        <div style={{ padding: "0 16px 8px", flex: "none" }}>
          <span
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 6,
              height: 26,
              padding: "0 6px 0 10px",
              borderRadius: 999,
              background: "var(--brand-subtle)",
              border: "1px solid var(--border-focus)",
              color: "var(--brand)",
              fontSize: 12,
              fontWeight: 600,
            }}
          >
            role: {peer.roleFilter}
            <button
              onClick={peer.clearRole}
              aria-label="Clear role filter"
              style={{ display: "flex", alignItems: "center", justifyContent: "center", width: 18, height: 18, border: 0, borderRadius: "50%", background: "rgba(37,99,253,.16)", color: "var(--brand)", cursor: "pointer" }}
            >
              <CloseIcon size={12} />
            </button>
          </span>
        </div>
      )}

      {/* Rows — infinite scroll loads the next keyset page near the bottom. */}
      <div
        style={{ flex: 1, overflowY: "auto", minHeight: 0 }}
        onScroll={(e) => {
          const el = e.currentTarget;
          if (el.scrollHeight - el.scrollTop - el.clientHeight < 160) void peer.loadMore();
        }}
      >
        {rows.map((c) => {
          const mixed = peer.isMixed(c);
          return (
            <button
              key={c.id}
              data-testid="peer-row"
              data-conversation={c.id}
              onClick={() => void peer.setActive(c.id)}
              style={{
                display: "flex",
                gap: 11,
                width: "100%",
                textAlign: "left",
                padding: "13px 14px",
                border: 0,
                borderBottom: "1px solid var(--border-subtle)",
                background: c.id === peer.activeId ? "var(--brand-subtle)" : "transparent",
                cursor: "pointer",
                alignItems: "flex-start",
              }}
            >
              <AvatarStack people={c.participants.map((p) => ({ name: p.name }))} size={36} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span style={{ flex: 1, minWidth: 0, fontSize: 14, fontWeight: 600, color: "var(--text-primary)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                    {peer.title(c)}
                  </span>
                  <span style={{ fontSize: 11, color: "var(--text-tertiary)", flex: "none" }}>{c.time}</span>
                </div>
                <div style={{ fontSize: 13, color: "var(--text-secondary)", marginTop: 2, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                  {c.preview}
                </div>
                <div style={{ fontSize: 11, color: "var(--text-tertiary)", fontFamily: "var(--font-mono)", marginTop: 3, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                  {c.reference}
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 7, marginTop: 7 }}>
                  <span
                    style={{
                      display: "inline-flex",
                      alignItems: "center",
                      height: 18,
                      padding: "0 7px",
                      borderRadius: 999,
                      fontSize: 10,
                      fontWeight: 700,
                      letterSpacing: ".02em",
                      textTransform: "uppercase",
                      color: mixed ? "var(--green-600)" : "var(--blue-600)",
                      background: mixed ? "var(--green-50)" : "var(--blue-50)",
                    }}
                  >
                    {peer.kindLabel(c)}
                  </span>
                  {c.joined && (
                    <span style={{ display: "inline-flex", alignItems: "center", gap: 3, fontSize: 11, fontWeight: 600, color: "var(--coral-600)" }}>
                      ✓ You joined
                    </span>
                  )}
                  <div style={{ flex: 1 }} />
                  {c.unread > 0 && (
                    <span
                      data-testid="peer-unread"
                      style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", minWidth: 18, height: 18, padding: "0 5px", borderRadius: 9, background: "var(--coral-500)", color: "#fff", fontSize: 11, fontWeight: 700 }}
                    >
                      {c.unread}
                    </span>
                  )}
                </div>
              </div>
            </button>
          );
        })}
        {peer.loaded && rows.length === 0 && (
          <div style={{ padding: "40px 24px", textAlign: "center", color: "var(--text-tertiary)", fontSize: 13 }}>
            {peer.searching ? "Searching…" : "No peer conversations in this view."}
          </div>
        )}
        {peer.loadingMore && (
          <div style={{ padding: "12px", textAlign: "center", color: "var(--text-tertiary)", fontSize: 12 }}>Loading…</div>
        )}
      </div>
    </div>
  );
});
