package io.sild.core

import kotlin.test.Test
import kotlin.test.assertEquals

// Pure test for clock(): a backend timestamp must render in the device's zone (not
// its original offset), matching the web client. The zone is passed explicitly — the
// device default is not settable on every platform. Common, so the JVM and Apple
// implementations are held to identical output.
class ClockTest {
    private val newYork = "America/New_York"
    private val tokyo = "Asia/Tokyo"
    private val utc = "UTC"

    @Test fun convertsUtcInstantToDeviceZone() {
        // 2026-07-15T16:30:00Z is 12:30 in New York (EDT, −4) and 01:30 the next day in Tokyo (+9).
        assertEquals("12:30 PM", clock("2026-07-15T16:30:00Z", newYork))
        assertEquals("1:30 AM", clock("2026-07-15T16:30:00Z", tokyo))
    }

    @Test fun honorsExplicitOffsetAsAnInstant() {
        // Same instant expressed with a +02:00 offset must still convert to the device zone.
        assertEquals("12:30 PM", clock("2026-07-15T18:30:00+02:00", newYork))
        assertEquals("12:30 PM", clock("2026-07-15T14:30:00-02:00", newYork))
    }

    @Test fun acceptsVariableFractionalSeconds() {
        assertEquals("12:30 PM", clock("2026-07-15T16:30:00.123Z", newYork))
        assertEquals("12:30 PM", clock("2026-07-15T16:30:00.123456789Z", newYork))
    }

    @Test fun formatsTwelveHourWithNoLeadingZero() {
        assertEquals("12:00 AM", clock("2026-07-15T00:00:00Z", utc), "midnight is 12 AM")
        assertEquals("12:05 PM", clock("2026-07-15T12:05:00Z", utc), "noon is 12 PM")
        assertEquals("9:07 AM", clock("2026-07-15T09:07:00Z", utc), "the hour drops its leading zero")
        assertEquals("11:59 PM", clock("2026-07-15T23:59:00Z", utc))
    }

    @Test fun blankOrUnparseableIsEmpty() {
        assertEquals("", clock(null, newYork))
        assertEquals("", clock("", newYork))
        assertEquals("", clock("   ", newYork))
        assertEquals("", clock("not-a-timestamp", newYork))
        // A local time names no instant, so it is not rendered.
        assertEquals("", clock("2026-07-15T16:30:00", newYork))
    }
}
