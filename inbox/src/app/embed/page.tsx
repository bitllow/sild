import { EmbeddedApp } from "@/components/inbox/EmbeddedApp";
import { StoreProvider } from "@/store/StoreProvider";

// Framed by a host app; middleware.ts sets frame-ancestors from the embed key.
export default function EmbedPage() {
  return (
    <StoreProvider bootstrap={false}>
      <EmbeddedApp />
    </StoreProvider>
  );
}
