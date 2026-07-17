package expo.modules.radarlegacytoken

import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

class RadarLegacyTokenModule : Module() {
  private fun preferences() = EncryptedSharedPreferences.create(
    appContext.reactContext ?: error("React context is unavailable"),
    LEGACY_PREFERENCES_NAME,
    MasterKey.Builder(appContext.reactContext ?: error("React context is unavailable"))
      .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
      .build(),
    EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
    EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
  )

  override fun definition() = ModuleDefinition {
    Name("RadarLegacyToken")

    AsyncFunction("readLegacyTokenAsync") {
      preferences().getString(LEGACY_TOKEN_KEY, null)
    }

    AsyncFunction("clearLegacyTokenAsync") {
      preferences().edit().remove(LEGACY_TOKEN_KEY).commit()
    }
  }

  private companion object {
    const val LEGACY_PREFERENCES_NAME = "radar_secure_prefs"
    const val LEGACY_TOKEN_KEY = "access_token"
  }
}
