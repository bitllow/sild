import { issueUpload, uploadUrl } from "@sild/client";
import type { PendingAttachment } from "@/store/types";
import { http } from "./client";

// putUpload issues a signed direct-to-bucket PUT (§11) and sends the file, so
// the local-dev URL rewrite lives in exactly one place. Returns the object key.
export async function putUpload(file: File, mime: string): Promise<string> {
  const grant = await issueUpload(http, { mime_type: mime, size_bytes: file.size, filename: file.name });
  // The local backend's URL goes same-origin through the Next proxy, with the
  // auth cookie; a cloud signed URL is used as is.
  const { url, local } = uploadUrl(http, grant);
  const res = await fetch(url, {
    method: "PUT",
    body: file,
    headers: { "Content-Type": mime },
    credentials: local ? "include" : "omit",
  });
  if (!res.ok) throw new Error("upload failed");
  return grant.object_key;
}

// uploadFile returns a reference to attach to the next message. Shared by every
// composer (support + peer).
export async function uploadFile(file: File): Promise<PendingAttachment> {
  const mime = file.type || "application/octet-stream";
  return {
    objectKey: await putUpload(file, mime),
    disposition: mime.startsWith("image/") ? "inline" : "attachment",
    mimeType: mime,
    filename: file.name,
  };
}
