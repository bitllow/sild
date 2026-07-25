package io.sild.core

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.add
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

/** Thrown for a non-2xx REST response; message is the server's `error.message`. */
class SildApiException(val status: Int, message: String) : RuntimeException(message)

// SildApi is the §4.2 REST surface (user-JWT), matching the web client's api()
// helper: it prefixes {base}/v1, sends Authorization: Bearer, and on a 401
// refreshes the token exactly once and retries. Blocking OkHttp calls run on the
// IO dispatcher; every method is suspend.
internal class SildApi(private val cfg: SildConfig) {
    private val http = SildHttp.client
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }
    private val jsonMedia = "application/json".toMediaType()

    @Volatile
    private var token: String? = null

    private suspend fun bearer(force: Boolean = false): String {
        val t = token
        if (t != null && !force) return t
        return cfg.tokenProvider.token().also { token = it }
    }

    // api sends a request with a fresh-enough bearer token, retrying once on 401
    // with a forced token refresh (expired/rotated). Returns the raw body string
    // ("" on 204). Throws SildApiException on non-2xx.
    private suspend fun api(method: String, path: String, body: String? = null): String =
        withContext(Dispatchers.IO) {
            fun send(tok: String): okhttp3.Response {
                val builder = Request.Builder()
                    .url(cfg.base + "/v1" + path)
                    .header("Authorization", "Bearer $tok")
                    // API surface only — an extra header can break a signed upload URL.
                    .header("X-Sild-SDK", "android/$SDK_VERSION")
                val reqBody = body?.toRequestBody(jsonMedia)
                when (method) {
                    "GET" -> builder.get()
                    "POST" -> builder.post(reqBody ?: "".toRequestBody(jsonMedia))
                    else -> error("unsupported method $method")
                }
                return http.newCall(builder.build()).execute()
            }

            var res = send(bearer())
            if (res.code == 401) {
                res.close()
                res = send(bearer(force = true)) // expired/rotated — refresh once
            }
            res.use {
                val text = it.body?.string().orEmpty()
                if (it.code == 204) return@withContext ""
                if (!it.isSuccessful) throw SildApiException(it.code, errorMessage(text) ?: it.message)
                text
            }
        }

    private fun errorMessage(text: String): String? = runCatching {
        json.parseToJsonElement(text).jsonObject["error"]?.jsonObject?.get("message")?.jsonPrimitive?.content
    }.getOrNull()

    // ── endpoints ────────────────────────────────────────────────────────────

    /** GET /v1/brands/active → { name, config }. Same URL the web drop-in uses;
     *  a credential scopes it to our tenant.
     *  The logo URL is re-based onto our base so it loads from any host (see rebaseLocalUrl). */
    suspend fun fetchBrand(): BrandResponse {
        val res: BrandResponse = json.decodeFromString(api("GET", "/brands/active"))
        return res.copy(config = res.config.copy(logoUrl = rebaseLocalUrl(cfg.base, res.config.logoUrl)))
    }

    /** GET /v1/conversations → the standard list envelope. The credential scopes
     *  it to the caller's own conversations, so there is no /me variant. */
    suspend fun listConversations(): List<ApiConversation> =
        json.decodeFromString<ApiConversationsPage>(api("GET", "/conversations")).items

    /** GET /v1/conversations/{id}/messages?limit=100 → the standard list envelope. */
    suspend fun listMessages(id: String): ApiMessagesPage =
        json.decodeFromString(api("GET", "/conversations/$id/messages?limit=100"))

    /** POST /v1/conversations { metadata } → { id }. open_assignment is forced
     *  true for a user credential, so this always opens a support request. */
    suspend fun openSupportRequest(): String {
        val body = buildJsonObject {
            put("metadata", JsonObject(cfg.metadata.mapValues { JsonPrimitive(it.value) }))
        }
        val obj = json.parseToJsonElement(api("POST", "/conversations", body.toString())).jsonObject
        return obj.getValue("id").jsonPrimitive.content
    }

    /** POST /v1/conversations/{id}/messages { body, client_msg_id, attachments } → ApiMessage. */
    suspend fun sendMessage(id: String, text: String, clientMsgId: String, attachments: List<PendingAttachment>): ApiMessage {
        val body = buildJsonObject {
            put("body", text)
            put("client_msg_id", clientMsgId)
            putJsonArray("attachments") {
                attachments.forEach {
                    add(buildJsonObject {
                        put("object_key", it.objectKey)
                        put("disposition", it.disposition)
                    })
                }
            }
        }
        return json.decodeFromString(api("POST", "/conversations/$id/messages", body.toString()))
    }

    /** POST /v1/uploads then PUT the file to the signed URL (with the local-dev rewrite). */
    suspend fun upload(bytes: ByteArray, filename: String, mimeType: String): PendingAttachment {
        val mime = mimeType.ifEmpty { "application/octet-stream" }
        val grantBody = buildJsonObject {
            put("mime_type", mime)
            put("size_bytes", bytes.size)
            put("filename", filename)
        }
        val grant: ApiUploadGrant = json.decodeFromString(api("POST", "/uploads", grantBody.toString()))
        // Local dev returns an absolute public-origin URL; re-base its /v1 path onto
        // our own base so the PUT works from any host. Real cloud signed URLs pass through.
        val putUrl = rebaseLocalUrl(cfg.base, grant.uploadUrl)!!
        withContext(Dispatchers.IO) {
            val req = Request.Builder().url(putUrl)
                .put(bytes.toRequestBody(mime.toMediaType()))
                .build()
            http.newCall(req).execute().use { if (!it.isSuccessful) throw SildApiException(it.code, "upload failed") }
        }
        return PendingAttachment(
            objectKey = grant.objectKey,
            disposition = if (mime.startsWith("image/")) "inline" else "attachment",
            mimeType = mime,
            filename = filename,
        )
    }
}
