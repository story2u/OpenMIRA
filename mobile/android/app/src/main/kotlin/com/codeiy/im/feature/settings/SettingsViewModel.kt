package com.codeiy.im.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.codeiy.im.core.network.RadarApi
import com.codeiy.im.core.network.api
import com.codeiy.im.model.DetectionSettings
import com.codeiy.im.model.DetectionSettingsUpdate
import com.codeiy.im.model.NotificationSettings
import com.codeiy.im.model.NotificationSettingsUpdate
import com.codeiy.im.model.SettingsBundle
import com.codeiy.im.model.WorkSchedule
import com.codeiy.im.model.WorkScheduleSlotDTO
import com.codeiy.im.model.WorkScheduleUpdate
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

class SettingsViewModel(private val service: RadarApi) : ViewModel() {
    data class UiState(
        val bundle: SettingsBundle? = null,
        val isLoading: Boolean = false,
        val loadError: String? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state

    fun load() {
        _state.value = _state.value.copy(isLoading = true)
        viewModelScope.launch {
            try {
                _state.value = _state.value.copy(bundle = api { service.settings() }, isLoading = false, loadError = null)
            } catch (e: Exception) {
                // 加载失败不显示默认值冒充服务端值。
                _state.value = _state.value.copy(isLoading = false, loadError = e.message)
            }
        }
    }

    /** 乐观更新 + 失败回滚：立即改本地，失败恢复旧值并回调错误。 */
    fun saveDetection(keywords: List<String>, aiSemanticsEnabled: Boolean, onError: (String) -> Unit) {
        val bundle = _state.value.bundle ?: return
        val previous = bundle.detection
        _state.value = _state.value.copy(bundle = bundle.copy(detection = DetectionSettings(keywords, aiSemanticsEnabled)))
        viewModelScope.launch {
            try {
                val saved = api { service.updateDetection(DetectionSettingsUpdate(keywords, aiSemanticsEnabled)) }
                _state.value = _state.value.copy(bundle = _state.value.bundle?.copy(detection = saved))
            } catch (e: Exception) {
                _state.value = _state.value.copy(bundle = _state.value.bundle?.copy(detection = previous))
                onError(e.message ?: "保存失败")
            }
        }
    }

    fun saveWorkSchedule(
        timezone: String,
        slots: List<WorkScheduleSlotDTO>,
        autoReplyOutsideHours: Boolean,
        onError: (String) -> Unit,
    ) {
        val bundle = _state.value.bundle ?: return
        val previous = bundle.workSchedule
        _state.value = _state.value.copy(
            bundle = bundle.copy(
                workSchedule = WorkSchedule(timezone, slots, autoReplyOutsideHours, false)
            )
        )
        viewModelScope.launch {
            try {
                val saved = api {
                    service.updateWorkSchedule(
                        WorkScheduleUpdate(timezone, slots, autoReplyOutsideHours)
                    )
                }
                _state.value = _state.value.copy(bundle = _state.value.bundle?.copy(workSchedule = saved))
            } catch (e: Exception) {
                _state.value = _state.value.copy(bundle = _state.value.bundle?.copy(workSchedule = previous))
                onError(e.message ?: "保存失败")
            }
        }
    }

    fun saveNotifications(prefs: NotificationSettings, onError: (String) -> Unit) {
        val bundle = _state.value.bundle ?: return
        val previous = bundle.notifications
        _state.value = _state.value.copy(bundle = bundle.copy(notifications = prefs))
        viewModelScope.launch {
            try {
                val saved = api {
                    service.updateNotifications(
                        NotificationSettingsUpdate(
                            prefs.newOpportunityEnabled,
                            prefs.aiRepliedEnabled,
                            prefs.dailyDigestEnabled,
                            prefs.urgentOnly,
                        )
                    )
                }
                _state.value = _state.value.copy(bundle = _state.value.bundle?.copy(notifications = saved))
            } catch (e: Exception) {
                _state.value = _state.value.copy(bundle = _state.value.bundle?.copy(notifications = previous))
                onError(e.message ?: "保存失败")
            }
        }
    }
}
