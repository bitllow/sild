import { makeAutoObservable, runInAction } from "mobx";
import { uploadFile } from "@/api/upload";
import type { AttachmentRef } from "@/api/admin";
import type { PendingAttachment } from "./types";

// AttachmentQueue owns the "files queued for the next message" state that every
// composer shares (support + peer): direct-to-bucket uploads (§11), an in-flight
// counter the composer waits on, and a generation guard so an upload that lands
// AFTER the active conversation switched is discarded rather than leaking into a
// different thread. One implementation, composed by each store — not copy-pasted.
export class AttachmentQueue {
  pending: PendingAttachment[] = [];
  uploading = 0;
  private gen = 0;

  // onError surfaces an upload failure to the owning store (e.g. an inline banner).
  constructor(private onError?: (msg: string) => void) {
    makeAutoObservable(this);
  }

  /** Clear the queue and invalidate any in-flight uploads — call on conv switch. */
  reset() {
    this.pending = [];
    this.uploading = 0;
    this.gen += 1;
  }

  /** Upload each file and queue a reference for the next message. */
  attach(files: File[]) {
    const gen = this.gen;
    for (const file of files) {
      this.uploading += 1;
      void uploadFile(file)
        .then((att) =>
          runInAction(() => {
            if (this.gen === gen) this.pending.push(att);
          })
        )
        .catch((e) =>
          runInAction(() => {
            if (this.gen === gen) this.onError?.(e instanceof Error ? e.message : `Could not upload ${file.name}.`);
          })
        )
        .finally(() =>
          runInAction(() => {
            if (this.gen === gen) this.uploading -= 1;
          })
        );
    }
  }

  remove(i: number) {
    this.pending.splice(i, 1);
  }

  get isUploading(): boolean {
    return this.uploading > 0;
  }

  /** The queued attachments as message refs, WITHOUT clearing — so a failed send
   *  leaves them in place to retry. Call clear() only after the send succeeds. */
  refs(): AttachmentRef[] {
    return this.pending.map((a) => ({ object_key: a.objectKey, disposition: a.disposition }));
  }

  /** Empty the queue (after a successful send). */
  clear() {
    this.pending = [];
  }
}
