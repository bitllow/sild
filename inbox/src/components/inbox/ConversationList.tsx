"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Button, CloseIcon, ConversationRow, SearchIcon, Select, SpeakerIcon, SpeakerOffIcon, StatusPill } from "@/components/ds";
import type { QueueSort } from "@/api/admin";
import { closedToggleStyle, filterStyle, soundBtnStyle, tabCountStyle } from "./styles";

// Passed via Button's `iconLeft` so it's a separate flex child (gap-spaced,
// vertically centered) rather than sharing the label's inline text run.
const PlusInline = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ display: "block" }}>
    <path d="M5 12h14M12 5v14" />
  </svg>
);

const SORT_OPTIONS: { value: QueueSort; label: string }[] = [
  { value: "last_activity", label: "Last activity" },
  { value: "created", label: "Date started" },
  { value: "waiting_since", label: "Waiting since" },
];

// A single chevron that points down for descending, up for ascending.
const SortDirIcon = ({ desc }: { desc: boolean }) => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ transform: desc ? "none" : "rotate(180deg)" }}>
    <path d="M12 5v14M19 12l-7 7-7-7" />
  </svg>
);

export const ConversationList = observer(function ConversationList() {
  const store = useStore();
  const rows = store.listConvs;
  const searchActive = store.searchResults !== null;

  // Scroll-loading: fetch the next page as the list nears the bottom (§4.3).
  const onScroll = (e: React.UIEvent<HTMLDivElement>) => {
    if (searchActive) return;
    const el = e.currentTarget;
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 240) {
      void store.loadMore();
    }
  };

  return (
    <div
      style={{
        width: 360,
        flex: "none",
        borderRight: "1px solid var(--border-default)",
        background: "var(--surface-card)",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <div style={{ padding: "16px 16px 12px" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
          <h2 style={{ fontSize: 18 }}>Inbox</h2>
          <div style={{ display: "flex", alignItems: "center", gap: 8, flex: "none" }}>
            <button
              data-testid="sound-toggle"
              aria-pressed={store.soundOn}
              aria-label={store.soundOn ? "Mute new-conversation sounds" : "Unmute new-conversation sounds"}
              title={store.soundOn ? "Mute new-conversation sounds" : "Unmute new-conversation sounds"}
              onClick={store.toggleSound}
              style={soundBtnStyle(store.soundOn)}
            >
              {store.soundOn ? <SpeakerIcon size={18} /> : <SpeakerOffIcon size={18} />}
            </button>
            <Button size="sm" onClick={store.newRequest} iconLeft={<PlusInline />}>
              New request
            </Button>
          </div>
        </div>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            height: 34,
            background: "var(--white)",
            border: "1px solid var(--border-default)",
            borderRadius: 8,
            padding: "0 10px",
            marginTop: 12,
          }}
        >
          <SearchIcon size={16} stroke="var(--text-tertiary)" />
          <input
            placeholder="Search — try status:open or a phone number"
            value={store.searchQuery}
            onChange={(e) => store.setSearchQuery(e.target.value)}
            style={{
              flex: 1,
              border: 0,
              outline: "none",
              background: "transparent",
              fontFamily: "var(--font-sans)",
              fontSize: 13,
              color: "var(--text-primary)",
              minWidth: 0,
            }}
          />
          {store.searchQuery && (
            <button
              onClick={() => store.setSearchQuery("")}
              aria-label="Clear search"
              style={{ border: 0, background: "transparent", cursor: "pointer", color: "var(--text-tertiary)", display: "flex", padding: 0 }}
            >
              <CloseIcon size={15} />
            </button>
          )}
        </div>
        <div
          style={{
            display: "flex",
            gap: 4,
            marginTop: 12,
            background: "var(--surface-sunken)",
            padding: 3,
            borderRadius: 8,
          }}
        >
          <button data-testid="filter-you" aria-pressed={store.filter === "you"} onClick={() => store.setFilter("you")} style={filterStyle(store.filter === "you")}>
            You
            {store.youCount > 0 && <span style={tabCountStyle()}>{store.youCount}</span>}
          </button>
          <button data-testid="filter-unassigned" aria-pressed={store.filter === "unassigned"} onClick={() => store.setFilter("unassigned")} style={filterStyle(store.filter === "unassigned")}>
            Unassigned
            {store.unassignedCount > 0 && <span style={tabCountStyle()}>{store.unassignedCount}</span>}
          </button>
          <button data-testid="filter-all" aria-pressed={store.filter === "all"} onClick={() => store.setFilter("all")} style={filterStyle(store.filter === "all")}>
            All
          </button>
        </div>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, marginTop: 10 }}>
          <span data-testid="open-count" style={{ fontSize: 12, color: "var(--text-tertiary)", whiteSpace: "nowrap" }}>
            {store.openCount} open
          </span>
          {store.closedCount > 0 && (
            <button
              data-testid="toggle-closed"
              aria-pressed={store.showClosed}
              onClick={store.toggleClosed}
              style={closedToggleStyle(store.showClosed)}
            >
              {store.showClosed ? "Hide closed" : `Show closed · ${store.closedCount}`}
            </button>
          )}
        </div>
        {store.authorFilter && (
          <div
            data-testid="contact-filter-chip"
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: 8,
              marginTop: 10,
              padding: "7px 10px",
              background: "var(--brand-subtle)",
              border: "1px solid var(--border-focus)",
              borderRadius: 8,
            }}
          >
            <span style={{ fontSize: 12, color: "var(--text-primary)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              All from <strong>{store.authorFilter}</strong> ({store.authorCount})
            </span>
            <button
              onClick={store.clearAuthorFilter}
              aria-label="Clear contact filter"
              style={{ display: "flex", alignItems: "center", gap: 4, border: 0, background: "transparent", cursor: "pointer", color: "var(--brand)", fontFamily: "var(--font-sans)", fontSize: 12, fontWeight: 600, flex: "none" }}
            >
              Clear
              <CloseIcon size={13} />
            </button>
          </div>
        )}
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 10 }}>
          <span style={{ fontSize: 12, color: "var(--text-tertiary)", flex: "none" }}>Sort</span>
          <div style={{ flex: 1, minWidth: 0 }}>
            <Select
              size="sm"
              options={SORT_OPTIONS}
              value={store.sortBy}
              onChange={(e) => store.setSort(e.target.value as QueueSort)}
            />
          </div>
          <button
            onClick={store.toggleSortDir}
            aria-label={store.sortDir === "desc" ? "Sort descending" : "Sort ascending"}
            title={store.sortDir === "desc" ? "Newest first" : "Oldest first"}
            style={{
              width: 34,
              height: 34,
              flex: "none",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              border: "1px solid var(--border-default)",
              background: "var(--white)",
              borderRadius: 8,
              cursor: "pointer",
              color: "var(--text-secondary)",
            }}
          >
            <SortDirIcon desc={store.sortDir === "desc"} />
          </button>
        </div>
      </div>
      <div
        onScroll={onScroll}
        style={{ flex: 1, overflowY: "auto", borderTop: "1px solid var(--border-subtle)" }}
      >
        {rows.map((c) => (
          <ConversationRow
            key={c.id}
            data-testid="conversation-row"
            data-conversation={c.id}
            name={c.name}
            preview={c.preview}
            time={c.time}
            unread={c.unread}
            channel={c.channel}
            reference={c.reference}
            presence={c.presence}
            active={c.id === store.activeId}
            onClick={() => store.setActive(c.id)}
            status={c.status === "queued" ? <StatusPill status="queued" /> : null}
          />
        ))}
        {rows.length === 0 && (
          <div style={{ padding: "40px 24px", textAlign: "center", color: "var(--text-tertiary)", fontSize: 13, lineHeight: 1.6 }}>
            {searchActive
              ? store.searching
                ? "Searching…"
                : `No matches for "${store.searchQuery}".`
              : "No conversations in this view. New support requests land here the moment they're assigned."}
          </div>
        )}
        {!searchActive && store.loadingMore && (
          <div style={{ padding: "14px 24px", textAlign: "center", color: "var(--text-tertiary)", fontSize: 12 }}>
            Loading more…
          </div>
        )}
      </div>
    </div>
  );
});
