package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.plugins.timeout
import io.ktor.client.request.header
import io.ktor.client.request.put
import io.ktor.client.request.request
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.client.statement.bodyAsText
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.content.ByteArrayContent
import io.ktor.http.content.TextContent
import io.ktor.http.encodeURLParameter
import io.ktor.http.isSuccess
import kotlin.concurrent.Volatile
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.add
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray

/** How many conversations the messenger's recent list asks for. */
internal const val RECENT_CONVERSATIONS = 20

/** Thrown for a non-2xx REST response; message is the server's `error.message`. */
class SildApiException(val status: Int, message: String) : RuntimeException(message)

// SildApi is the §4.2 REST surface (user-JWT), matching the web client's api()
// helper: it prefixes {base}/v1, sends Authorization: Bearer, and on a 401
// refreshes the token exactly once and retries. Every method is suspend.
internal class SildApi(
    private val cfg: SildConfig,
    private val http: HttpClient = SildHttp.client,
) {
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

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
    private suspend fun api(method: String, path: String, body: String? = null): String {
        suspend fun send(tok: String): HttpResponse = http.request(cfg.base + "/v1" + path) {
            this.method = HttpMethod.parse(method)
            header(HttpHeaders.Authorization, "Bearer $tok")
            // API surface only — an extra header can break a signed upload URL.
            header("X-Sild-SDK", "android/$SDK_VERSION")
            if (method == "POST") setBody(TextContent(body.orEmpty(), ContentType.Application.Json))
        }

        var res = send(bearer())
        if (res.status.value == 401) res = send(bearer(force = true)) // expired/rotated — refresh once
        val text = res.bodyAsText()
        if (res.status.value == 204) return ""
        if (!res.status.isSuccess()) throw SildApiException(res.status.value, errorMessage(text) ?: res.status.description)
        return text
    }

    private fun errorMessage(text: String): String? = runCatching {
        json.parseToJsonElement(text).jsonObject["error"]?.jsonObject?.get("message")?.jsonPrimitive?.content
    }.getOrNull()

    // ── endpoints ────────────────────────────────────────────────────────────

    /** GET /v1/brands/active → { name, config }; a credential scopes it to our
     *  tenant. The logo URL is re-based so it loads from any host. */
    suspend fun fetchBrand(): BrandResponse {
        val res: BrandResponse = json.decodeFromString(api("GET", "/brands/active"))
        return res.copy(config = res.config.copy(logoUrl = rebaseLocalUrl(cfg.base, res.config.logoUrl)))
    }

    /** GET /v1/conversations?limit= → the most recent rows, scoped by credential.
     *  One bounded page: the messenger shows a short recent list with no paging, and
     *  this also runs on every reconnect. */
    suspend fun listConversations(limit: Int = RECENT_CONVERSATIONS): List<ApiConversation> {
        val page: ApiConversationsPage = json.decodeFromString(api("GET", "/conversations?limit=$limit"))
        return page.items
    }

    /** GET /v1/conversations/{id}/messages?limit=100 → the standard list envelope. */
    suspend fun listMessages(id: String): ApiMessagesPage =
        json.decodeFromString(api("GET", "/conversations/$id/messages?limit=100"))

    /** GET /v1/conversations/{id}/messages?since={id} → messages after [since],
     *  OLDEST first. A sync read, not a page: next_cursor is always null and
     *  has_more means "call again with the last id you got". */
    suspend fun catchUpMessages(id: String, since: String, limit: Int = 100): ApiMessagesPage =
        json.decodeFromString(
            api("GET", "/conversations/$id/messages?since=${since.encodeURLParameter()}&limit=$limit"),
        )

    /** POST /v1/conversations { metadata } → { id }; always a support request. */
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
        // The bucket verifies a signature over the request shape, so the PUT carries
        // the bytes and nothing else — no API headers.
        val res = http.put(putUrl) {
            setBody(ByteArrayContent(bytes, ContentType.parse(mime)))
            signedShape()
            timeout { socketTimeoutMillis = UPLOAD_SOCKET_TIMEOUT_MS }
        }
        if (!res.status.isSuccess()) throw SildApiException(res.status.value, "upload failed")
        return PendingAttachment(
            objectKey = grant.objectKey,
            disposition = if (mime.startsWith("image/")) "inline" else "attachment",
            mimeType = mime,
            filename = filename,
        )
    }
}
