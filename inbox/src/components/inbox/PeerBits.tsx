import type { PeerStore } from "@/store/peer";

// RolePill renders a participant's tenant-defined role, colored by the peer
// store's derived role→color ordering (first party role = blue, second = green,
// support/agent = coral). Label = the role string itself, never hardcoded.
export function RolePill({ role, isAgent, peer }: { role: string; isAgent?: boolean; peer: PeerStore }) {
  if (!role) return null;
  const s = peer.roleStyle(role, isAgent);
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        height: 19,
        padding: "0 8px",
        borderRadius: 999,
        fontFamily: "var(--font-sans)",
        fontSize: 11,
        fontWeight: 700,
        letterSpacing: ".01em",
        color: s.color,
        background: s.bg,
      }}
    >
      {role}
    </span>
  );
}
