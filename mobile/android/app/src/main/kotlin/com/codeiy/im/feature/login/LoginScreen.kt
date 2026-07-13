package com.codeiy.im.feature.login

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.credentials.CredentialManager
import androidx.credentials.GetCredentialRequest
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialException
import com.codeiy.im.BuildConfig
import com.codeiy.im.R
import com.codeiy.im.core.auth.SessionStore
import com.google.android.libraries.identity.googleid.GetGoogleIdOption
import com.google.android.libraries.identity.googleid.GoogleIdTokenCredential
import kotlinx.coroutines.launch

/** 校验规则对齐 iOS LoginView：@ 切两段、域名含点、邮箱 ≤320、密码 1..128。 */
private fun emailLooksValid(email: String): Boolean {
    val parts = email.split("@").filter { it.isNotEmpty() }
    return parts.size == 2 && parts[1].contains(".") && email.length <= 320
}

/** P0 登录：邮箱密码 + Google 原生登录（`GOOGLE_SERVER_CLIENT_ID` 未配置时隐藏 Google 按钮）。 */
@Composable
fun LoginScreen(session: SessionStore) {
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var isSigningIn by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val focusManager = LocalFocusManager.current
    val passwordFocus = remember { FocusRequester() }

    val canSubmit = !isSigningIn &&
        emailLooksValid(email.trim()) &&
        password.isNotEmpty() && password.length <= 128

    fun submitPasswordLogin() {
        if (!canSubmit) return
        focusManager.clearFocus()
        isSigningIn = true
        error = null
        scope.launch {
            try {
                session.signIn(email, password)
            } catch (e: Exception) {
                error = e.message ?: "登录失败"
            } finally {
                isSigningIn = false
            }
        }
    }

    fun submitGoogleLogin() {
        isSigningIn = true
        error = null
        scope.launch {
            try {
                val option = GetGoogleIdOption.Builder()
                    .setServerClientId(BuildConfig.GOOGLE_SERVER_CLIENT_ID)
                    .setFilterByAuthorizedAccounts(false)
                    .build()
                val request = GetCredentialRequest.Builder().addCredentialOption(option).build()
                val credential = CredentialManager.create(context)
                    .getCredential(context, request)
                    .credential
                val idToken = GoogleIdTokenCredential.createFrom(credential.data).idToken
                session.signInWithGoogle(idToken)
            } catch (e: GetCredentialCancellationException) {
                // 用户取消，不算错误
            } catch (e: GetCredentialException) {
                error = "Google 登录不可用：${e.message ?: e.type}"
            } catch (e: Exception) {
                error = e.message ?: "登录失败"
            } finally {
                isSigningIn = false
            }
        }
    }

    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Image(
            painter = painterResource(R.drawable.brand_logo),
            contentDescription = "商机雷达",
            modifier = Modifier
                .height(56.dp)
                .clip(RoundedCornerShape(12.dp)),
        )
        Spacer(Modifier.height(12.dp))
        Text("商机雷达", style = MaterialTheme.typography.headlineLarge)
        Spacer(Modifier.height(4.dp))
        Text(
            "Telegram / 企业微信商机，推送到手，随手处理",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(32.dp))

        OutlinedTextField(
            value = email,
            onValueChange = { email = it },
            label = { Text("邮箱") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Email,
                imeAction = ImeAction.Next,
                autoCorrectEnabled = false,
            ),
            keyboardActions = KeyboardActions(onNext = { passwordFocus.requestFocus() }),
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(
            value = password,
            onValueChange = { password = it },
            label = { Text("密码") },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Password,
                imeAction = ImeAction.Go,
            ),
            keyboardActions = KeyboardActions(onGo = { submitPasswordLogin() }),
            modifier = Modifier.fillMaxWidth().focusRequester(passwordFocus),
        )
        error?.let {
            Spacer(Modifier.height(8.dp))
            Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
        }
        Spacer(Modifier.height(20.dp))
        Button(
            onClick = { submitPasswordLogin() },
            enabled = canSubmit,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (isSigningIn) {
                CircularProgressIndicator(modifier = Modifier.height(20.dp))
            } else {
                Text("登录")
            }
        }

        if (BuildConfig.GOOGLE_SERVER_CLIENT_ID.isNotEmpty()) {
            Spacer(Modifier.height(16.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                HorizontalDivider(Modifier.weight(1f))
                Text(
                    "或",
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 12.dp),
                )
                HorizontalDivider(Modifier.weight(1f))
            }
            Spacer(Modifier.height(16.dp))
            OutlinedButton(
                onClick = { submitGoogleLogin() },
                enabled = !isSigningIn,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text("使用 Google 登录")
            }
        }
    }
}
