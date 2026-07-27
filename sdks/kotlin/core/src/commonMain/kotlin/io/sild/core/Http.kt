package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.HttpClientConfig
import io.ktor.client.engine.HttpClientEngineFactory
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.plugins.HttpTimeoutConfig
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.HttpRequestPipeline
import io.ktor.http.HttpHeaders
import io.ktor.util.AttributeKey
import io.ktor.util.pipeline.PipelinePhase

/** Attachment PUTs send the whole file in one request, so they get a longer socket budget. */
internal const val UPLOAD_SOCKET_TIMEOUT_MS = 60_000L

private val SIGNED_SHAPE = AttributeKey<Unit>("SildSignedShape")

/** Mark a request whose URL is signed over its shape — nothing may be added to it. */
internal fun HttpRequestBuilder.signedShape() {
    attributes.put(SIGNED_SHAPE, Unit)
}

internal expect val sildHttpEngine: HttpClientEngineFactory<*>

// One HTTP client per process: its connection pool and worker threads are meant to
// be shared, not rebuilt per messenger launch. Lazy so an unused SDK costs nothing.
internal object SildHttp {
    val client: HttpClient by lazy { HttpClient(sildHttpEngine) { sildDefaults() } }
}

internal fun HttpClientConfig<*>.sildDefaults() {
    expectSuccess = false
    install(HttpTimeout) {
        connectTimeoutMillis = 15_000
        socketTimeoutMillis = 30_000
        // No overall call deadline; the socket budget is what bounds a request.
        requestTimeoutMillis = HttpTimeoutConfig.INFINITE_TIMEOUT_MS
    }
    // The default transformers append Accept: */* after the request is built; a
    // signed request must carry only its content framing.
    install("SildSignedShape") {
        val phase = PipelinePhase("SildSignedShape")
        requestPipeline.insertPhaseAfter(HttpRequestPipeline.Render, phase)
        requestPipeline.intercept(phase) {
            if (context.attributes.contains(SIGNED_SHAPE)) context.headers.remove(HttpHeaders.Accept)
        }
    }
}
