import { makeAutoObservable, runInAction } from "mobx";
import { ApiError } from "@sild/client";
import { adminApi, type ApiIdentityProvider, type IdentityProviderInput } from "@/api/admin";
import { errorText } from "@/api/client";

/** Settings → Embedding: the tenant's embedded sign-in config. */
export class EmbeddingStore {
  /** null when embedded sign-in is off. */
  provider: ApiIdentityProvider | null = null;
  loaded = false;
  busy = false;
  message: { text: string; kind: "error" | "ok" } | null = null;

  constructor() {
    makeAutoObservable(this);
  }

  load = async () => {
    try {
      const provider = await adminApi.getIdentityProvider();
      runInAction(() => (this.provider = provider));
    } catch (e) {
      runInAction(() => {
        if (e instanceof ApiError && e.status === 404) this.provider = null;
        else this.message = { text: errorText(e), kind: "error" };
      });
    } finally {
      runInAction(() => (this.loaded = true));
    }
  };

  save = (input: IdentityProviderInput) =>
    this.run(async () => {
      const provider = await adminApi.putIdentityProvider(input);
      runInAction(() => (this.provider = provider));
    }, "Saved.");

  turnOff = () =>
    this.run(async () => {
      await adminApi.deleteIdentityProvider();
      runInAction(() => (this.provider = null));
    }, "Embedded sign-in is off.");

  private run = async (fn: () => Promise<void>, notice: string): Promise<boolean> => {
    runInAction(() => {
      this.busy = true;
      this.message = null;
    });
    try {
      await fn();
      runInAction(() => (this.message = { text: notice, kind: "ok" }));
      return true;
    } catch (e) {
      runInAction(() => (this.message = { text: errorText(e), kind: "error" }));
      return false;
    } finally {
      runInAction(() => (this.busy = false));
    }
  };
}
