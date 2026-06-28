"use client";

import * as React from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Button, GoogleMark } from "@/components/ds";

const fieldStyle: React.CSSProperties = {
  width: "100%",
  height: 44,
  border: "1px solid var(--border-default)",
  borderRadius: 8,
  padding: "0 12px",
  fontFamily: "var(--font-sans)",
  fontSize: 14,
  color: "var(--text-primary)",
  background: "var(--white)",
  outline: "none",
};

export const Login = observer(function Login() {
  const store = useStore();
  const [email, setEmail] = React.useState("admin@sild.local");
  const [password, setPassword] = React.useState("");

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || !password || store.authBusy) return;
    void store.loginPassword(email.trim(), password);
  };

  return (
    <div
      style={{
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "var(--surface-page)",
      }}
    >
      <div
        style={{
          width: 380,
          background: "var(--surface-card)",
          border: "1px solid var(--border-default)",
          borderRadius: 16,
          boxShadow: "var(--shadow-lg)",
          padding: 36,
          textAlign: "center",
          animation: "sild-rise .25s ease-out",
        }}
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src="/assets/sild-mark-tile.svg" width={46} alt="Sild" style={{ borderRadius: 13 }} />
        <h1 style={{ fontSize: 24, marginTop: 18, letterSpacing: "-0.02em" }}>Sild support inbox</h1>
        <p style={{ fontSize: 14, color: "var(--text-secondary)", marginTop: 8, lineHeight: 1.5 }}>
          Sign in to the assignment queue. Agents see only the conversations they&apos;re assigned.
        </p>

        <form onSubmit={submit} style={{ marginTop: 22, display: "flex", flexDirection: "column", gap: 10, textAlign: "left" }}>
          <input
            type="email"
            autoComplete="username"
            placeholder="you@company.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            style={fieldStyle}
          />
          <input
            type="password"
            autoComplete="current-password"
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            style={fieldStyle}
          />
          {store.authError && (
            <div style={{ fontSize: 13, color: "var(--danger)", lineHeight: 1.4 }}>{store.authError}</div>
          )}
          <Button type="submit" fullWidth loading={store.authBusy} disabled={store.authBusy}>
            Sign in
          </Button>
        </form>

        <div style={{ display: "flex", alignItems: "center", gap: 10, margin: "18px 0" }}>
          <span style={{ flex: 1, height: 1, background: "var(--border-default)" }} />
          <span style={{ fontSize: 12, color: "var(--text-tertiary)" }}>or</span>
          <span style={{ flex: 1, height: 1, background: "var(--border-default)" }} />
        </div>

        <button
          onClick={store.loginGoogle}
          type="button"
          style={{
            width: "100%",
            height: 46,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            gap: 10,
            border: "1px solid var(--border-default)",
            background: "var(--white)",
            borderRadius: 8,
            fontFamily: "var(--font-sans)",
            fontSize: 15,
            fontWeight: 600,
            color: "var(--text-primary)",
            cursor: "pointer",
          }}
        >
          <GoogleMark />
          Continue with Google
        </button>
        <p style={{ fontSize: 12, color: "var(--text-tertiary)", marginTop: 18 }}>
          Admin identity is separate from chat end-users.
        </p>
      </div>
    </div>
  );
});
