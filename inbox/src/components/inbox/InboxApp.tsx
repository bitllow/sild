"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Login } from "./Login";
import { Shell } from "./Shell";

export const InboxApp = observer(function InboxApp() {
  const store = useStore();
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100vh",
        width: "100%",
        overflow: "hidden",
        background: "var(--surface-page)",
      }}
    >
      {store.session === "loading" ? (
        <Loading />
      ) : store.session === "anon" ? (
        <Login />
      ) : (
        <Shell />
      )}
    </div>
  );
});

function Loading() {
  return (
    <div style={{ height: "100%", display: "flex", alignItems: "center", justifyContent: "center" }}>
      <span
        className="sild-spinner"
        role="status"
        aria-label="Loading"
        style={{ width: 28, height: 28, borderWidth: 3 }}
      />
    </div>
  );
}
