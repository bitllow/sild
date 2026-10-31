import { uploadFile as upload } from "@sild/client";
import type { PendingAttachment } from "@/store/types";
import { http } from "./client";

// Shared by every composer (support + peer). The local backend's URL goes
// same-origin through the Next proxy, with the auth cookie.
export const uploadFile = (file: File): Promise<PendingAttachment> => upload(http, file, "include");
