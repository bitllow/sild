package io.sild.core

import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertEquals

// Pure parse/derivation test on a captured /me/conversations payload — isolates
// wire mapping (member metadata → name) from the live backend.
class MappingTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test fun memberNameParses() {
        val body = """
        {"conv_role":"driver","external_user_id":"u_driver_x","member_kind":"user",
         "name":"Toomas Vaher"}
        """.trimIndent()
        val m = json.decodeFromString(ApiMember.serializer(), body)
        assertEquals("Toomas Vaher", m.name)
    }
}
