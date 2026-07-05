package io.sild.sample

import io.sild.core.TokenProvider
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.OkHttpClient
import okhttp3.Request

// DevBackend stands in for the host app's own backend, exactly like web/demo.html:
// it mints a user token and (for the driver chat) asks the dev server to ensure a
// peer conversation. A real host would mint tokens with its API key server-side
// and create peer conversations via POST /v1/conversations (open_assignment:false).
//
// BASE defaults to the Android emulator's alias for the host machine's localhost
// (10.0.2.2), where `make -C backend run` / sild-dev listens on :8080.
object DevBackend {
    const val BASE = "http://10.0.2.2:8080"
    const val USER_ID = "u_demo_android"

    private val http = OkHttpClient()
    private val json = Json { ignoreUnknownKeys = true }

    /** A TokenProvider hitting the dev token-mint endpoint (host stand-in). */
    val tokenProvider = TokenProvider {
        withContext(Dispatchers.IO) {
            val req = Request.Builder().url("$BASE/v1/dev/widget-token?user_id=$USER_ID").build()
            http.newCall(req).execute().use {
                json.parseToJsonElement(it.body!!.string()).jsonObject.getValue("token").jsonPrimitive.content
            }
        }
    }

    /** Ensure the trip's driver↔rider peer conversation exists; return its id. */
    suspend fun ensureDriverConversation(reference: String): String = withContext(Dispatchers.IO) {
        val req = Request.Builder().url("$BASE/v1/dev/peer-conversation?user_id=$USER_ID&reference=$reference").build()
        http.newCall(req).execute().use {
            json.parseToJsonElement(it.body!!.string()).jsonObject.getValue("conversation_id").jsonPrimitive.content
        }
    }
}
