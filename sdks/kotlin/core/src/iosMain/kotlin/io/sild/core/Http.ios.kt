package io.sild.core

import io.ktor.client.engine.HttpClientEngineFactory
import io.ktor.client.engine.darwin.Darwin

internal actual val sildHttpEngine: HttpClientEngineFactory<*> = Darwin
