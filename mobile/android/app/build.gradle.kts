plugins {
    id("com.android.application")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.opene2ee.opene2ee"
    // Sprint 11.0B — flutter_webrtc 0.13.x requires compileSdk >= 36.
    // The flutter.compileSdkVersion default is 35; we override to 36
    // here so the plugin's `peer_connection_factory` native ABI loads
    // without a `:app:compileDebugKotlin` warning cascade. The
    // targetSdk stays at flutter.targetSdkVersion (35) so the
    // Android 14+ foreground service type variety behavior (Sprint
    // 11.0A) is unchanged.
    compileSdk = 36
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "com.opene2ee.opene2ee"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    buildTypes {
        release {
            // TODO: Add your own signing config for the release build.
            // Signing with the debug keys for now, so `flutter run --release` works.
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    // Sprint 11.0Z — exclude META-INF/INDEX.LIST from
    // the APK packaging. The `io.netty:netty-all:4.1.107.Final`
    // bundle is an all-in-one JAR that contains
    // `META-INF/INDEX.LIST` from EVERY Netty module
    // (netty-buffer, netty-codec, netty-handler,
    // netty-transport, netty-resolver-dns, etc.).
    // Android's `mergeDebugJavaResource` task refuses
    // to merge 34 jars with the same `META-INF/INDEX.LIST`
    // file — it would silently overwrite each other.
    // `pickFirst` tells Gradle to use the first
    // occurrence and skip the rest.
    packaging {
        resources {
            excludes += setOf("META-INF/INDEX.LIST", "META-INF/io.netty.versions.properties")
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}

// Sprint 11.0Z — user-space TCP/IP stack via Netty.
// The pre-11.0Z code did transparent passthrough
// (write the IP packet back to the TUN output and
// let the kernel route it). Owner 22:08 root cause:
// the kernel cannot route the packet because the
// OpenE2ee VPN does not own a real network interface
// — the captured packets have no corresponding
// outbound socket. The fix is a user-space
// TCP/IP stack: read IP packets from the TUN, parse
// the IP+TCP/UDP headers, create a real socket to
// the destination (with `VpnService.protect(socket)`
// so the socket bypasses the VPN and uses the real
// NIC), and forward the data bidirectionally.
// Netty provides the async NIO socket layer;
// the IP/TCP/UDP header parsing is done in
// `NettyChannelClient.kt` (user-space protocol
// stack). `io.netty:netty-all:4.1.107.Final` is
// the all-in-one bundle (transport + buffer +
// codec + handler). 4.1.107 is the current stable
// (Nov 2023). S99 audit verifies the dep is
// declared + `VpnService.protect` is called + the
// `NettyChannelClient` class is present.
dependencies {
    implementation("io.netty:netty-all:4.1.107.Final")
}
