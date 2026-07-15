package io.sild.core

import java.util.TimeZone
import kotlin.test.Test
import kotlin.test.assertEquals

// Pure test for clock(): a UTC backend timestamp must render in the device's local
// zone (not its original offset), matching the web client. Drives ZoneId.systemDefault()
// by swapping the JVM default TimeZone, restoring it afterwards.
class ClockTest {
    private fun withZone(id: String, block: () -> Unit) {
        val prev = TimeZone.getDefault()
        try {
            TimeZone.setDefault(TimeZone.getTimeZone(id))
            block()
        } finally {
            TimeZone.setDefault(prev)
        }
    }

    @Test fun convertsUtcInstantToDeviceZone() {
        // 2026-07-15T16:30:00Z is 12:30 in New York (EDT, −4) and 01:30 the next day in Tokyo (+9).
        withZone("America/New_York") { assertEquals("12:30 PM", clock("2026-07-15T16:30:00Z")) }
        withZone("Asia/Tokyo") { assertEquals("1:30 AM", clock("2026-07-15T16:30:00Z")) }
    }

    @Test fun honorsExplicitOffsetAsAnInstant() {
        // Same instant expressed with a +02:00 offset must still convert to the device zone.
        withZone("America/New_York") { assertEquals("12:30 PM", clock("2026-07-15T18:30:00+02:00")) }
    }

    @Test fun blankOrUnparseableIsEmpty() {
        assertEquals("", clock(null))
        assertEquals("", clock(""))
        assertEquals("", clock("not-a-timestamp"))
    }
}
