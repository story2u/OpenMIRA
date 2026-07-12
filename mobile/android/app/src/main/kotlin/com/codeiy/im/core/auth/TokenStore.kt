package com.codeiy.im.core.auth

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/** JWT 加密存储（对齐 iOS Keychain）：不进普通 SharedPreferences 明文/日志。 */
class TokenStore(context: Context) {
    private val prefs by lazy {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            "radar_secure_prefs",
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    var token: String?
        get() = prefs.getString(KEY, null)
        set(value) {
            if (value == null) prefs.edit().remove(KEY).apply()
            else prefs.edit().putString(KEY, value).apply()
        }

    private companion object {
        const val KEY = "access_token"
    }
}
