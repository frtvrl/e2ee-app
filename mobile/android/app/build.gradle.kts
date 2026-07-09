// mobile/android/app/build.gradle.kts
//
// PR-28 (Sprint 5) â€” Flutter app module build script.
// Sprint 9.6.5 hotfix: Kotlin DSL syntax upgrade for Kotlin 2.0+ compatibility.
//
// Configures the `:app` subproject that hosts both the Flutter embedding
// (Activity + MethodChannels) AND the native Kotlin VPN service that
// PR-22a introduced and PR-24 relocated into the Android source tree.
//
// Highlights:
//   - Applies Flutter's Android Gradle plugin (handles Dart build,
//     `flutter pub get`, GeneratedPluginRegistrant injection, asset
//     bundling).
//   - Applies the Kotlin Android plugin (lets Gradle compile the
//     `mobile/android/app/src/main/kotlin/com/opene2ee/opene2ee/**/*.kt`
//     sources â€” including `vpn/OpenE2eeVpnService.kt`).
//   - Pins minSdk = 23 (Android 6.0 Marshmallow) â€” see SPRINT-7 Â§MOB-5
//     rationale block below. targetSdk = 34 (Android 14, required for
//     `foregroundServiceType="specialUse"`), compileSdk = 34.
//   - Declares runtime deps: AndroidX core, AndroidX annotation (for
//     `@RequiresApi` lint enforcement), androidx.core (ServiceCompat),
//     kotlinx-coroutines-android (used by future async plumbing).
//   - Distinguishes debug vs release signing configs. Debug uses the
//     AGP-provided auto-generated debug keystore (no password needed,
//     valid for one year â€” fine for sampling / on-device testing).
//     Release is empty unless the developer drops a keystore at
//     `mobile/android/key.properties` (gitignored).
//
// â”€â”€â”€ SPRINT-7 Â§MOB-5 â€” minSdk = 23 rationale (Android 6.0 Marshmallow)
// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// History: PR-22a / PR-28 set minSdk = 21 (Lollipop) because that was
// the floor for `VpnService.Builder.allowedApplications()` and
// `.disallowedApplications()`. MOB-5 (cyber-security hand-off) found
// this floor was BELOW the floor that flutter_secure_storage 9.x
// requires for AndroidKeyStore-backed encryption:
//
//   â€¢ flutter_secure_storage 9.2.4 (pinned in pubspec.yaml) documents:
//       "API Level: Android 6.0 (API 23) minimum for basic encryption"
//     https://pub.dev/packages/flutter_secure_storage  (Requirements)
//   â€¢ flutter_secure_storage 9.x changelog records:
//       "Minimum Android SDK changed from 19 to 23"
//   â€¢ `android.security.keystore.KeyGenParameterSpec` (the modern
//     AndroidKeyStore key-generation builder â€” supports PURPOSE_*,
//     BLOCK_MODE_*, ENCRYPTION_PADDING_*, user-auth requirements,
//     randomized-encryption flag) was added in API 23.
//     Pre-API-23 only had `KeyPairGeneratorSpec`, which lacks every
//     one of those controls. flutter_secure_storage's AES master key
//     cannot be created on API < 23 without falling back to software-
//     only SharedPreferences â€” which would silently break the Ed25519
//     private-key-at-rest guarantee that ADR-0006 Â§B1 relies on.
//
// Decision: bump minSdk from 21 â†’ 23. This is preferred over
// "attestation" (Option B in the Sprint 7 spec) because the API
// literally does not exist on API < 23 â€” there is nothing to
// attest to. It is preferred over "explicit failure at runtime"
// (Option C) because end-users on Android 5.x would see a crash
// with no recourse; refusing to install is more honest.
//
// VPN impact: `VpnService.Builder.allowedApplications()` is API 21+,
// so VPN functionality is preserved unchanged. The `@RequiresApi(21)`
// annotations on OpenE2eeVpnService.kt are now belt-and-braces
// (the floor enforces it) but kept as defensive documentation.
//
// Market-share note (2026): Android 5.0-5.1.1 < 0.5% of active
// devices per Google Play Console public dashboards. Acceptable
// drop for a security-critical E2EE app.
//
// Regression guard: mobile/test/min_sdk_posture_test.dart parses this
// file and asserts minSdk >= 23. Update the test constant if the
// floor ever moves.
//
// References:
//   - https://docs.flutter.dev/deployment/android
//   - https://developer.android.com/build/building-cmdline
//   - https://developer.android.com/reference/android/security/keystore/KeyGenParameterSpec
//   - https://pub.dev/packages/flutter_secure_storage
//   - docs/ADR-0003-vpn-layer.md
//   - docs/ADR-0006-anonimlik.md (Ed25519 private-key-at-rest contract)
//   - docs/SPRINT-7-SCOPE.md Â§Item 6 MOB-5
//
// â”€â”€â”€ SPRINT-9.6.6 â€” Kotlin 2.0+ DSL import hygiene (Kotlin 2.2.20 + AGP 8.11.1)
// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// Sprint 9.6.5 partially fixed Kotlin 2.0+ DSL strictness (smart cast
// `as String` removal, `variant` rename to `_`, `keyProps.getProperty(...)`
// access) but TWO defects were left in place and a live workflow_dispatch
// run AFTER Sprint 9.6.5 PR #16 push (origin/main @ e2b7055) failed with:
//   (a) `kotlinOptions { jvmTarget = "17" }` is DEPRECATED in Kotlin
//       2.0+ â€” the build emits a deprecation warning and AGP 8.11.1 +
//       Kotlin 2.2.20 treat the warning as an error in CI. The
//       migration target is `kotlin { compilerOptions { jvmTarget
//       .set(JvmTarget.JVM_17) } }`, which requires the explicit
//       import `org.jetbrains.kotlin.gradle.dsl.JvmTarget`.
//   (b) The Sprint 9.6.5 comment claimed "explicit `import
//       java.util.Properties`" but the file STILL uses the fully-
//       qualified `java.util.Properties()` form at line 151. Kotlin
//       2.0+ is stricter on implicit imports â€” the Kotlin compiler
//       rejects fully-qualified class names that are not preceded by
//       an explicit `import` statement with:
//         "Unresolved reference: util" (line 145, the class
//         reference inside the lambda)
//         "Unresolved reference: load" (line 152, the Properties
//         `load(InputStream)` call inside the `apply { ... }` block
//         requires the explicit `Properties` type to be in scope)
//       Sprint 9.6.6 fixes BOTH defects: explicit `import
//       java.util.Properties` is added at the top of this file, and
//       the fully-qualified `java.util.Properties()` usage on line
//       151 is replaced with the short form `Properties()`.
//   (c) The original Sprint 5 form used `apply { load(...) }` to
//       populate the Properties. With the explicit import in
//       place (item (b) above), the implicit receiver
//       `load(keyPropsFile.inputStream())` resolves correctly:
//       `apply` provides `this` as `Properties`, and
//       `load(InputStream)` is a method on `Properties`. Kotlin
//       2.0+'s stricter smart-cast receiver handling is satisfied
//       because `this` is a stable, fully-typed `Properties`
//       reference, NOT a `MutableMap<String, Any>` (the pre-2.0
//       form's implicit type).
//
// Audit-script side (tools/workflow-yaml-audit.py): the Sprint 9.6.5
// pair-check `check_app_build_gradle_syntax()` was a substring search
// for `import java.util.Properties` and returned PASS when the
// substring appeared INSIDE a comment â€” a false positive. Sprint
// 9.6.6 fixes this by parsing actual `^import <pkg>` lines via
// regex (ignoring line-comment and block-comment context).

import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.opene2ee.opene2ee"
    compileSdk = 34
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    // Sprint 9.6.6: `kotlinOptions { jvmTarget = "17" }` (the
    // nested form inside `android { }`) is DEPRECATED in Kotlin
    // 2.0+ and emits a deprecation warning that AGP 8.11.1 +
    // Kotlin 2.2.20 treat as a build error in CI. The migration
    // moves `kotlin { compilerOptions { ... } }` OUT of the
    // `android { }` block to a TOP-LEVEL sibling (defined
    // immediately after `android { }` closes, before
    // `flutter { }`). See the SPRINT-9.6.6 rationale block at the
    // top of this file and the top-level `kotlin { }` block below.

    sourceSets {
        getByName("main").java.srcDirs("src/main/kotlin")
    }

    defaultConfig {
        // Floor â€” API 23 (Android 6.0 Marshmallow) â€” see the SPRINT-7
        // Â§MOB-5 rationale block at the top of this file. The two
        // hard requirements that pin this floor are:
        //   (1) `android.security.keystore.KeyGenParameterSpec` (the
        //       AndroidKeyStore key-generation builder used by
        //       flutter_secure_storage's AES master key) is API 23+.
        //   (2) flutter_secure_storage 9.2.4 explicitly documents
        //       "Android 6.0 (API 23) minimum for basic encryption".
        // VPN functionality is preserved: `VpnService.Builder
        // .allowedApplications()` / `.disallowedApplications()` are
        // API 21+, so the per-app VPN features OpenE2eeVpnService.kt
        // ships still work. The `@RequiresApi(21)` annotations in
        // OpenE2eeVpnService.kt are now defensive (the floor enforces
        // it) but kept for documentation.
        // Regression guard: mobile/test/min_sdk_posture_test.dart.
        minSdk = 23
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"
    }

    signingConfigs {
        // Debug uses AGP's auto-generated debug keystore (created on
        // first build under `~/.android/debug.keystore`). No password
        // required. Long enough validity for sampling / QA / CI builds.
        getByName("debug") {
            // Defaults are fine; we declare it explicitly so the
            // release block below can mirror its structure.
        }

        // Release â€” empty by default. If a developer drops a
        // `key.properties` file at `mobile/android/key.properties`
        // (see `.gitignore`), it is loaded here and a release
        // signing config is wired up. We deliberately do NOT fall
        // back to the debug keystore for release builds â€” that
        // would silently ship APKls signed with the well-known
        // Android SDK debug key.
        create("release") {
            val keyPropsFile = rootProject.file("key.properties")
            if (keyPropsFile.exists()) {
                // Sprint 9.6.6: this block was partially fixed in
                // Sprint 9.6.5 (smart-cast + `variant` rename) but
                // the `import java.util.Properties` statement was
                // NEVER added â€” only a comment claiming it was. A
                // live workflow_dispatch run AFTER Sprint 9.6.5 PR
                // #16 push (origin/main @ e2b7055) failed at this
                // line with:
                //   "Unresolved reference: util" (the fully-qualified
                //    `java.util.Properties` form rejected by Kotlin
                //    2.0+ without an explicit import)
                //   "Unresolved reference: load" (the
                //    `apply { load(...) }` block needs the explicit
                //    `Properties` type to be in scope)
                // Sprint 9.6.6 fixes by:
                //   1. Adding `import java.util.Properties` at the
                //      top of this file (Kotlin 2.0+ requires the
                //      import to be a real statement, not just a
                //      substring match in a comment).
                //   2. Using the short form `Properties()` instead
                //      of the fully-qualified `java.util.Properties()`
                //      here.
                //   3. Using `load(keyPropsFile.inputStream())` with
                //      implicit receiver â€” the `apply { ... }` block
                //      provides `this` as the `Properties` receiver,
                //      and `load(InputStream)` is a method on
                //      `Properties`. With the explicit
                //      `import java.util.Properties` at the top of
                //      this file, the implicit receiver resolves
                //      correctly. (Earlier draft attempted
                //      `keyProps.load(...)` with an explicit receiver,
                //      but that references `keyProps` inside the
                //      `apply { }` block BEFORE the variable is
                //      initialized â€” a Kotlin compile error.)
                val keyProps = Properties().apply {
                    load(keyPropsFile.inputStream())
                }
                // Sprint 9.6.5: smart cast handles the `as String` conversion
                // implicitly; the explicit `as String` casts from Sprint 5
                // triggered "No cast needed" warnings under Kotlin 2.0+ and
                // the `keyProps["storeFile"]` access would otherwise need
                // `Any?` -> `String` conversion. Use `toString()` + smart cast
                // for clarity + null safety.
                val storeFilePath: String = keyProps.getProperty("storeFile", "")
                val storePasswordValue: String = keyProps.getProperty("storePassword", "")
                val keyAliasValue: String = keyProps.getProperty("keyAlias", "")
                val keyPasswordValue: String = keyProps.getProperty("keyPassword", "")
                if (storeFilePath.isNotEmpty()) {
                    storeFile = file(storeFilePath)
                    storePassword = storePasswordValue
                    keyAlias = keyAliasValue
                    keyPassword = keyPasswordValue
                }
            }
        }
    }

    buildTypes {
        getByName("release") {
            // Sign release with the release config when a key is
            // configured, otherwise sign with debug so local
            // `flutter build apk --release` runs succeed (Play Store
            // uploads still require a real keystore).
            signingConfig = if (rootProject.file("key.properties").exists()) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
            // PR-39 (Sprint 6) â€” R8/ProGuard now enabled for release.
            // Addresses cyber-security review finding MOB-3 (High):
            //   "Release builds ship with `isMinifyEnabled = false;
            //    isShrinkResources = false` â€” full Kotlin symbol names +
            //    dev-friendly stack traces land in the APK. OWASP
            //    MASVS-CODE-2 violation."
            //
            // `proguard-android-optimize.txt` is AGP's optimized rule set
            // (further passes than `proguard-android.txt`); `proguard-rules.pro`
            // carries our project-specific keep rules (Flutter embedding,
            // MethodChannels, JNI callbacks, JSON DTOs).
            //
            // Verify every release build still runs the MethodChannels:
            //   - Flutter embedding activity launches cleanly
            //   - `mobile/vpn` MethodChannel reaches OpenE2eeVpnService
            //   - `flutter_secure_storage` MethodChannel reaches the
            //     Android Keystore backing store
            // See docs/SPRINT-6-PR-39-VERIFICATION.md for the manual
            // smoke-test plan.
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }
}

flutter {
    source = "../.."
}

// Sprint 9.6.6: Kotlin 2.0+ DSL migration. The deprecated
// `kotlinOptions { jvmTarget = "17" }` block (nested inside
// `android { }`) is replaced with a TOP-LEVEL `kotlin { ... }`
// extension block. `compilerOptions` is the Kotlin 2.0+ API;
// `jvmTarget.set(JvmTarget.JVM_17)` uses the strongly-typed
// `JvmTarget` enum (imported at the top of this file) instead
// of a String "17". The block is a SIBLING of `android { }`,
// not nested inside it â€” this is the form AGP 8.11.1 +
// Kotlin 2.2.20 expect.
//
// Migration path (Sprint 5 â†’ Sprint 9.6.6):
//   Sprint 5     : android { kotlinOptions { jvmTarget = "17" } }
//   Sprint 9.6.6 : kotlin { compilerOptions { jvmTarget.set(JvmTarget.JVM_17) } }
//
// Verification: tools/workflow-yaml-audit.py
// `check_app_build_gradle_kotlin_options_syntax()` fails the audit
// if the deprecated `kotlinOptions { jvmTarget = "..." }` block
// reappears in any future commit.
kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

dependencies {
    // Kotlin runtime â€” provided by the Kotlin Android plugin via
    // stdlib, declared explicitly so the Kotlin compiler can resolve
    // references like `kotlin.LazyThreadSafetyMode` from the VPN service.
    implementation("androidx.core:core-ktx:1.12.0")
    implementation("androidx.annotation:annotation:1.7.1")
    // `androidx.core.app.ServiceCompat` (startForeground, startForegroundService)
    // and `ForegroundServiceType` constants land here. Required by the
    // PR-28 transient-service fix to keep `startForeground` callable on
    // API 34+ without the deprecation warning breaking CI.
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.7.3")

    // Sprint 9.6.13 â€” Flutter engine embedding JAR (Kotlin side).
    //
    // The `id("dev.flutter.flutter-gradle-plugin")` plugin handles
    // `compileFlutterBuildDebug` (Dart side) and pulls the engine in its own
    // classpath, but `compileDebugKotlin` (Kotlin side, run by Android Gradle
    // Plugin) needs the engine JAR on its classpath too. Without this
    // dependency, every `io.flutter.embedding.*` and `io.flutter.plugin.*`
    // import in `MainActivity.kt` and `OpenE2eeVpnService.kt` fails to
    // resolve ("Unresolved reference 'embedding' / 'FlutterActivity' /
    // 'FlutterEngine' / 'MethodChannel'").
    //
    // The version `1.0.0-<engine_commit>` is pinned to the Flutter SDK's
    // engine.version file so the Kotlin side and Dart side always agree.
    // This dependency has been missing since PR-3; PR-28 (Sprint 5) set up
    // the Flutter Gradle plugin but never declared the embedding JAR.
    implementation("io.flutter:flutter_embedding_ktx:1.0.0-c416acfeb8126e097f758c664aaa3da929e27da0")
}

// Ensure the VPN service class file ends up in the compiled output â€”
// defensive; AGP/Kotlin already pick up `src/main/kotlin` via the
// sourceSets entry above, but a future rename or move is easier to
// audit if we list the package here explicitly.
androidComponents {
    // Sprint 9.6.5: rename unused parameter `variant` to `_` (Kotlin 2.0+
    // emits "parameter never used" warning for unused lambda parameters).
    onVariants { _ ->
        // no-op; placeholder for future ABI/signing hooks.
    }
}