package io.sild.sample

import androidx.compose.ui.test.junit4.ComposeTestRule
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import io.sild.core.Direction
import io.sild.core.SildClient
import io.sild.core.SildConfig
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

// The Android SDK's end-to-end smoke: launch the real sample app on an emulator,
// open a support conversation through the actual SDK entry points, and round-trip
// a message through a live sild-dev. It exercises the whole stack a host app hits —
// Sild.init, SildMessenger.openSupportRequest → SildMessengerActivity, SildClient's
// REST + realtime, and the Compose UI — so a launch crash, a broken wire contract,
// or a Compose render failure fails CI instead of shipping.
//
// Requires a running sild-dev; the app reaches the runner's :8080 at 10.0.2.2:8080
// (DevBackend.BASE). Run locally with:
//   (backend) make dev
//   (sdk)     ./gradlew :sample:connectedDebugAndroidTest      # emulator/device attached
//
// createEmptyComposeRule (not the activity-bound rule) is used because the flow
// spans two activities — MainActivity, then the SDK's SildMessengerActivity — so
// assertions target whichever composition is in the foreground.
@RunWith(AndroidJUnit4::class)
class SildMessengerE2ETest {

    @get:Rule
    val compose = createEmptyComposeRule()

    @Test
    fun openSupport_and_sendMessage_roundTrips() {
        ActivityScenario.launch(MainActivity::class.java).use {
            // The sample rendered without crashing on launch (Sild.init ran).
            compose.awaitText("Acme Rides")
            compose.onNodeWithText("Open support").assertExists()

            // Open support — launches the SDK's messenger, which lands on Home
            // (welcome + New conversation + Recent), matching the web widget.
            compose.onNodeWithText("Open support").performClick()
            compose.awaitText("New conversation")

            // Start a draft conversation; the composer appears (thread screen rendered).
            compose.onNodeWithText("New conversation").performClick()
            compose.awaitText("Message…")

            // Round-trip a message: type, send, and assert it lands in the thread.
            val body = "e2e-hello"
            compose.onNodeWithContentDescription("Message input").performTextInput(body)
            compose.onNodeWithContentDescription("Send").performClick()
            compose.awaitText(body, substring = true)
        }
    }

    // Peer chat, both directions, on-device. The sample's "Message driver" card opens
    // the trip's rider↔driver peer conversation; a driver-side core client (the other
    // party) rounds a message through the live backend against the rider's real UI:
    //  1. rider sends from the Compose composer → the driver client receives it (proves
    //     the peer UI's outgoing path + openConversation wiring), then
    //  2. the driver sends a reply → it renders in the rider's Compose thread (proves
    //     the realtime incoming path all the way to a rendered bubble — which the
    //     JVM-only peerDerivationAndRealtimeRoundTrip can't see).
    @Test
    fun peerChat_roundTripsBothWays() {
        val ref = DevBackend.DRIVER_TRIP_REF
        val driverId = "u_driver_$ref" // sild-dev derives the driver from the reference.
        val driverScope = CoroutineScope(Dispatchers.IO + SupervisorJob())
        val nonce = System.nanoTime().toString(36)

        // Bring the driver online first so it's subscribed when the rider sends.
        val convId = runBlocking { DevBackend.ensureDriverConversation(ref) }
        val driver = SildClient(
            SildConfig(DevBackend.BASE, DevBackend.tokenProviderFor(driverId), userId = driverId),
            driverScope,
        )
        try {
            driver.start(convId)
            awaitClient(driver) { it.messages.isNotEmpty() }

            ActivityScenario.launch(MainActivity::class.java).use {
                // Rider opens the driver chat from the trip card.
                compose.awaitText("Message driver")
                compose.onNodeWithText("Message driver").performClick()
                compose.awaitText("Message…")

                // (1) rider → driver: sent from the real composer, received by the driver.
                val fromRider = "rider-$nonce"
                compose.onNodeWithContentDescription("Message input").performTextInput(fromRider)
                compose.onNodeWithContentDescription("Send").performClick()
                val received = awaitClient(driver) { st -> st.messages.any { it.body == fromRider } }
                    .messages.first { it.body == fromRider }
                // The rider is the other party on the driver's side → incoming.
                assertEquals("rider msg should be incoming to driver", Direction.IN, received.direction)

                // (2) driver → rider: reply lands in the rider's Compose thread over WS.
                val fromDriver = "driver-$nonce"
                driver.send(fromDriver)
                compose.awaitText(fromDriver, substring = true)
            }
        } finally {
            driver.destroy()
            driverScope.cancel()
        }
    }

    /** Poll a core SildClient's state until [predicate] holds (realtime isn't idling-tracked). */
    private fun awaitClient(
        client: SildClient,
        timeoutMs: Long = 30_000L,
        predicate: (io.sild.core.SildState) -> Boolean,
    ): io.sild.core.SildState {
        compose.waitUntil(timeoutMs) { predicate(client.state.value) }
        return client.state.value
    }
}

// Network round-trips (token mint, brand fetch, realtime connect, send) aren't
// tracked by Compose's idling, so poll for the expected text with a ceiling
// generous enough for a cold emulator + backend rather than relying on waitForIdle.
private fun ComposeTestRule.awaitText(
    text: String,
    substring: Boolean = false,
    timeoutMs: Long = 30_000L,
) = waitUntil(timeoutMs) {
    onAllNodesWithText(text, substring = substring).fetchSemanticsNodes().isNotEmpty()
}
