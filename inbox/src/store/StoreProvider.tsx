"use client";

import * as React from "react";
import { RootStore } from "./RootStore";

const StoreContext = React.createContext<RootStore | null>(null);

export function StoreProvider({ children }: { children: React.ReactNode }) {
  const store = React.useRef<RootStore | null>(null);
  if (!store.current) store.current = new RootStore();

  React.useEffect(() => {
    void store.current!.bootstrap();
    const s = store.current!;
    return () => s.dispose();
  }, []);

  return <StoreContext.Provider value={store.current}>{children}</StoreContext.Provider>;
}

export function useStore(): RootStore {
  const store = React.useContext(StoreContext);
  if (!store) throw new Error("useStore must be used within a StoreProvider");
  return store;
}
