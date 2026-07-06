package io.sild.ui

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.unit.dp

// SildIcons are the widget's Feather-style stroked glyphs, built from the exact SVG
// path data the web drop-in uses (web/src/widget/App.tsx) so the two surfaces render
// the same icon language — thin 2px round-capped strokes, not Material's filled set.
// Icon() applies its `tint` as a color filter over the whole vector, so the stroke
// color below is a placeholder the caller's tint overrides.
private fun feather(name: String, vararg paths: String): ImageVector {
    val b = ImageVector.Builder(
        name = name,
        defaultWidth = 24.dp, defaultHeight = 24.dp,
        viewportWidth = 24f, viewportHeight = 24f,
    )
    for (p in paths) {
        b.addPath(
            pathData = PathParser().parsePathString(p).toNodes(),
            fill = null,
            stroke = SolidColor(Color.Black),
            strokeLineWidth = 2f,
            strokeLineCap = StrokeCap.Round,
            strokeLineJoin = StrokeJoin.Round,
        )
    }
    return b.build()
}

object SildIcons {
    val Back = feather("Back", "M19 12H5M12 19l-7-7 7-7")
    val Send = feather("Send", "M22 2 11 13M22 2l-7 20-4-9-9-4 20-7")
    val Clip = feather("Clip", "M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48")
    val Arrow = feather("Arrow", "M5 12h14M12 5l7 7-7 7")
    val Chevron = feather("Chevron", "m9 18 6-6-6-6")
    val Close = feather("Close", "M18 6 6 18M6 6l12 12")
    val Speaker = feather("Speaker", "M11 4.7 6 9H2v6h4l5 4.3z", "M15.5 8.5a5 5 0 0 1 0 7M19 5a9 9 0 0 1 0 14")
    val SpeakerOff = feather("SpeakerOff", "M11 4.7 6 9H2v6h4l5 4.3z", "m22 9-6 6M16 9l6 6")
}
