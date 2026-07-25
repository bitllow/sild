# Keep rules the AAR hands to host apps, so their R8 doesn't strip what we reach
# reflectively. Exercised by :sample's minified release build.

# kotlinx.serialization — plugin-generated serializers for our @Serializable DTOs.
-keepattributes RuntimeVisibleAnnotations,AnnotationDefault,InnerClasses
-keepclassmembers class io.sild.core.** {
    *** Companion;
}
-keepclasseswithmembers class io.sild.core.** {
    kotlinx.serialization.KSerializer serializer(...);
}
-keep,includedescriptorclasses class io.sild.core.**$$serializer { *; }

# centrifuge-java — protocol frames are protobuf, built reflectively.
-keep class io.github.centrifugal.centrifuge.internal.protocol.** { *; }
-keep class com.google.protobuf.** { *; }
-dontwarn io.github.centrifugal.centrifuge.**
-dontwarn com.google.protobuf.**

# OkHttp — optional TLS providers a host won't ship.
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
