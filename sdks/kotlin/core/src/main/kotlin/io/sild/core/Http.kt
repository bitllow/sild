package io.sild.core

import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

// One OkHttp client per process: its pool and dispatcher threads are meant to be
// shared, not rebuilt per messenger launch. Lazy so an unused SDK costs nothing.
internal object SildHttp {
    val client: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            // Attachment PUTs send the whole file in one request.
            .writeTimeout(60, TimeUnit.SECONDS)
            .build()
    }
}
