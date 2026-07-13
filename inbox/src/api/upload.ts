import { adminApi } from "./admin";
import type { PendingAttachment } from "@/store/types";

// uploadFile issues a signed direct-to-bucket PUT (§11) and returns a reference
// to attach to the next message. Shared by every composer (support + peer) so the
// upload path — including the local-dev URL rewrite — lives in exactly one place.
export async function uploadFile(file: File): Promise<PendingAttachment> {
  const mime = file.type || "application/octet-stream";
  const grant = await adminApi.issueUpload(mime, file.size, file.name);
  // The local backend returns an absolute public-origin URL; PUT to its relative
  // /v1 path so it goes same-origin through the Next proxy (with the auth cookie).
  // Cloud signed URLs (no local route) are used as-is.
  const marker = "/v1/uploads/local/";
  const at = grant.upload_url.indexOf(marker);
  const putUrl = at >= 0 ? grant.upload_url.slice(at) : grant.upload_url;
  const res = await fetch(putUrl, {
    method: "PUT",
    body: file,
    headers: { "Content-Type": mime },
    credentials: at >= 0 ? "include" : "omit",
  });
  if (!res.ok) throw new Error("upload failed");
  return {
    objectKey: grant.object_key,
    disposition: mime.startsWith("image/") ? "inline" : "attachment",
    mimeType: mime,
    filename: file.name,
  };
}
