package io.sild.core

// Kotlin/Native cannot call SwiftCentrifuge (pure Swift, no ObjC surface), so the
// Swift layer implements RealtimeTransport and passes it to SildClient. Until it
// does, a client here is REST-only: no socket, and the connection stays IDLE.
actual fun defaultRealtimeTransport(): RealtimeTransportFactory? = null
