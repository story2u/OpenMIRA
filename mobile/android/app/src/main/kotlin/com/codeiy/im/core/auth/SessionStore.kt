package com.codeiy.im.core.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.codeiy.im.core.network.ApiClient
import com.codeiy.im.core.network.ApiError
import com.codeiy.im.core.network.api
import com.codeiy.im.model.AuthUser
import com.codeiy.im.model.PasswordLoginRequest
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/** 会话状态机，与 iOS SessionStore 对齐：恢复中 / 恢复失败 / 未登录 / 已登录。 */
sealed interface SessionState {
    data object Restoring : SessionState
    data class RestoreFailed(val message: String) : SessionState
    data object LoggedOut : SessionState
    data class Active(val user: AuthUser) : SessionState
}

class SessionStore(private val tokens: TokenStore) : ViewModel() {
    val api = ApiClient { tokens.token }

    private val _state = MutableStateFlow<SessionState>(SessionState.Restoring)
    val state: StateFlow<SessionState> = _state

    val currentUser: AuthUser?
        get() = (_state.value as? SessionState.Active)?.user

    init {
        restore()
    }

    /** 冷启动恢复：无 token 直接登出态；401 清 token；网络错误保留 token 允许重试。 */
    fun restore() {
        if (tokens.token == null) {
            _state.value = SessionState.LoggedOut
            return
        }
        _state.value = SessionState.Restoring
        viewModelScope.launch {
            try {
                _state.value = SessionState.Active(api { api.service.me() })
            } catch (e: ApiError.Unauthorized) {
                tokens.token = null
                _state.value = SessionState.LoggedOut
            } catch (e: Exception) {
                _state.value = SessionState.RestoreFailed(e.message ?: "会话恢复失败")
            }
        }
    }

    /** 邮箱密码登录（`POST /auth/password/login`）；凭据本身不落盘。 */
    suspend fun signIn(email: String, password: String) {
        val token = try {
            api { api.service.passwordLogin(PasswordLoginRequest(email.trim(), password)) }
        } catch (e: ApiError.Unauthorized) {
            throw ApiError.Server(401, "邮箱或密码错误")
        }
        tokens.token = token.accessToken
        _state.value = SessionState.Active(token.user)
    }

    /** 401 时由界面调用：清 token 回登录页（对齐验收「会话过期处理」）。 */
    fun logout() {
        tokens.token = null
        _state.value = SessionState.LoggedOut
    }
}
