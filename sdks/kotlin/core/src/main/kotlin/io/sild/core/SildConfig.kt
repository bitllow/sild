package io.sild.core

// TokenProvider mints a user JWT via the host backend (which holds the API key) —
// never the API key itself. A guest is the same call with a host-generated id.
// Suspend so the host can do a network call; the client caches the result and
// only re-invokes on a 401 or realtime token refresh (matches the web contract).
fun interface TokenProvider {
    suspend fun token(): String
}

// SildConfig is the public init contract, mirroring the web SildConfig. The host
// supplies a token provider and (optionally) its own user id + per-participant
// metadata; baseUrl points at the Sild backend.
data class SildConfig(
    val baseUrl: String,
    val tokenProvider: TokenProvider,
    /** The end-user's own id (the token subject) — lets the client tell the
     *  visitor's own messages from the other party's in a peer conversation.
     *  Optional; support-only usage doesn't need it. */
    val userId: String? = null,
    /** Per-participant metadata attached when this user opens a support request
     *  (becomes conversation_members.metadata, shown in the inbox). */
    val metadata: Map<String, String> = emptyMap(),
) {
    /** baseUrl with any trailing slash stripped (matches the web client). */
    val base: String get() = baseUrl.trimEnd('/')
}
