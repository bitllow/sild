"use client";

import * as React from "react";
import { RootStore } from "./RootStore";

const StoreContext = React.createContext<RootStore | null>(null);

// bootstrap=false leaves sign-in to the caller: the embedded inbox has no
// session until its host hands it a token.
export function StoreProvider({ children, bootstrap = true }: { children: React.ReactNode; bootstrap?: boolean }) {
  const store = React.useRef<RootStore | null>(null);
  if (!store.current) store.current = new RootStore();

  React.useEffect(() => {
    if (bootstrap) void store.current!.bootstrap();
    const s = store.current!;
    return () => s.dispose();
  }, [bootstrap]);

  return <StoreContext.Provider value={store.current}>{children}</StoreContext.Provider>;
}

export function useStore(): RootStore {
  const store = React.useContext(StoreContext);
  if (!store) throw new Error("useStore must be used within a StoreProvider");
  return store;
}
