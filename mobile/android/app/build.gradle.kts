import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

val revenueCatPublicApiKey = providers.gradleProperty("REVENUECAT_ANDROID_PUBLIC_API_KEY").orElse("")

android {
    namespace = "com.codeiy.im"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.codeiy.im"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
        buildConfigField("String", "REVENUECAT_ANDROID_PUBLIC_API_KEY", "\"${revenueCatPublicApiKey.get()}\"")

        // Google 原生登录用的 Web Client ID（非秘密）。在 gradle.properties 或 -P 提供；
        // 为空时登录页隐藏 Google 按钮，邮箱密码登录不受影响。
        val googleServerClientId = (project.findProperty("GOOGLE_SERVER_CLIENT_ID") as? String).orEmpty()
        buildConfigField("String", "GOOGLE_SERVER_CLIENT_ID", "\"$googleServerClientId\"")
    }

    buildTypes {
        debug {
            // 本地 compose 后端；模拟器用 10.0.2.2 访问宿主机。真机改成局域网地址或线上。
            buildConfigField("String", "API_BASE_URL", "\"http://10.0.2.2:8000/api/v1\"")
            isDebuggable = true
        }
        release {
            buildConfigField("String", "API_BASE_URL", "\"https://im.story2u.xyz/api/v1\"")
            isMinifyEnabled = false
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    val composeBom = platform(libs.compose.bom)
    implementation(composeBom)
    androidTestImplementation(composeBom)

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation("androidx.navigation:navigation-compose:2.8.5")

    // Google 原生登录：Credential Manager + Google ID Token
    implementation("androidx.credentials:credentials:1.3.0")
    implementation("androidx.credentials:credentials-play-services-auth:1.3.0")
    implementation("com.google.android.libraries.identity.googleid:googleid:1.1.1")

    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.extended)
    debugImplementation(libs.compose.ui.tooling)

    // 网络：Retrofit + kotlinx.serialization（对齐 iOS 唯一 HTTP 边界 + Codable 镜像）
    implementation(libs.retrofit.core)
    implementation(libs.retrofit.serialization)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)

    // JWT 加密存储：EncryptedSharedPreferences（对齐 iOS Keychain）
    implementation(libs.androidx.security.crypto)
    implementation(libs.revenuecat.purchases)

    testImplementation(libs.kotlinx.serialization.json)
    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.mockk)
}
