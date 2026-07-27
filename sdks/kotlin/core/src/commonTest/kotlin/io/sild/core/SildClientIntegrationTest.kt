package io.sild.core

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.withTimeout
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

// End-to-end tests against a live sild-dev (SILD_BASE_URL). They exercise the same
// wire contract the web widget uses; with SILD_BASE_URL unset each one returns
// immediately, so an offline run covers only the pure-logic tests.
class SildClientIntegrationTest {
    private val base get() = Dev.baseUrl!!

    private fun scope() = CoroutineScope(Dispatchers.Default + SupervisorJob())

    @Test fun fetchesBrand() = runBlockingTest {
        if (Dev.baseUrl == null) return@runBlockingTest
        val sc = scope()
        val client = SildClient(SildConfig(base, Dev.tokenProvider("u_brandtest")), sc)
        try {
            client.start()
            val s = awaitUntil(get = { client.state.value }) { it.ready && it.brand.brand.isNotEmpty() }
            assertTrue(s.brand.brand.startsWith("#"), "brand color should be a hex string, got ${s.brand.brand}")
            assertTrue(s.brand.heading.isNotEmpty())
        } finally {
            client.destroy(); sc.cancel()
        }
    }

    @Test fun supportRequestRoundTrip() = runBlockingTest {
        if (Dev.baseUrl == null) return@runBlockingTest
        val sc = scope()
        val user = uid("u_support")
        val client = SildClient(SildConfig(base, Dev.tokenProvider(user), userId = user), sc)
        try {
            client.start()
            awaitUntil(get = { client.state.value }) { it.ready }
            val body = "hello ${uid("m")}"
            client.openSupportRequest()
            awaitUntil(get = { client.state.value }) { it.activeId != null }
            // A successful send reports true (the composer clears the draft on this).
            val sendResult = CompletableDeferred<Boolean>()
            client.send(body) { sendResult.complete(it) }
            assertTrue(withTimeout(10_000) { sendResult.await() }, "send should report success")
            // Our own message is outgoing and appears in the thread.
            val s = awaitUntil(get = { client.state.value }) { st -> st.messages.any { it.body == body } }
            val sent = s.messages.first { it.body == body }
            assertEquals(Direction.OUT, sent.direction)
        } finally {
            client.destroy(); sc.cancel()
        }
    }

    @Test fun derivesPeerConversationFromTheList() = runBlockingTest {
        if (Dev.baseUrl == null) return@runBlockingTest
        val sc = scope()
        val rider = uid("u_rider")
        val ref = uid("trip")
        val convId = Dev.ensurePeerConversation(rider, ref)
        val client = SildClient(SildConfig(base, Dev.tokenProvider(rider), userId = rider), sc)
        try {
            // No assignment, no agent → peer.
            client.start(convId)
            val s = awaitUntil(get = { client.state.value }) {
                it.ready && it.messages.isNotEmpty() && it.conversations.any { c -> c.id == convId }
            }
            val conv = s.conversations.first { it.id == convId }
            assertTrue(conv.peer, "conversation should be derived as peer")
            assertEquals("Toomas Vaher", conv.title, "peer title = the other party (driver)")
            assertTrue(conv.subtitle!!.startsWith("Direct chat · $ref"), "subtitle=${conv.subtitle}")
            // The seeded driver message maps as incoming with the driver's name.
            val seeded = s.messages.last()
            assertEquals(Direction.IN, seeded.direction)
            assertEquals("Toomas Vaher", seeded.author)
        } finally {
            client.destroy(); sc.cancel()
        }
    }

    @Test fun realtimeDeliversTheOtherPartysMessage() = runBlockingTest {
        if (Dev.baseUrl == null) return@runBlockingTest
        // Platforms whose transport the host injects (iOS) have no socket here; the
        // REST half of the peer flow is covered by derivesPeerConversationFromTheList.
        if (defaultRealtimeTransport() == null) return@runBlockingTest
        val riderScope = scope()
        val driverScope = scope()
        val rider = uid("u_rider")
        val ref = uid("trip")
        val convId = Dev.ensurePeerConversation(rider, ref)
        val driver = Dev.driverId(ref)

        val riderClient = SildClient(SildConfig(base, Dev.tokenProvider(rider), userId = rider), riderScope)
        val driverClient = SildClient(SildConfig(base, Dev.tokenProvider(driver), userId = driver), driverScope)
        try {
            riderClient.start(convId)
            awaitUntil(get = { riderClient.state.value }) { it.ready && it.messages.isNotEmpty() }

            // Driver opens the same conversation; a rider message reaches it over WS.
            driverClient.start(convId)
            awaitUntil(get = { driverClient.state.value }) { it.connection == ConnectionState.CONNECTED && it.messages.isNotEmpty() }
            val ping = "ping ${uid("m")}"
            riderClient.send(ping)
            val ds = awaitUntil(timeoutMs = 15_000, get = { driverClient.state.value }) { st -> st.messages.any { it.body == ping } }
            val received = ds.messages.first { it.body == ping }
            // On the driver's side the rider is the other party → incoming, named.
            assertEquals(Direction.IN, received.direction)
            assertNotNull(received.author)
        } finally {
            riderClient.destroy(); driverClient.destroy()
            riderScope.cancel(); driverScope.cancel()
        }
    }
}
