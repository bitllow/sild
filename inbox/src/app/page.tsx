import { InboxApp } from "@/components/inbox/InboxApp";
import { StoreProvider } from "@/store/StoreProvider";

export default function Page() {
  return (
    <StoreProvider>
      <InboxApp />
    </StoreProvider>
  );
}
