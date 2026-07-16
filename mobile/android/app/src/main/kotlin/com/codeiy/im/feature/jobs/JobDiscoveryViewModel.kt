package com.codeiy.im.feature.jobs

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.codeiy.im.core.network.RadarApi
import com.codeiy.im.core.network.api
import com.codeiy.im.model.IMChannel
import com.codeiy.im.model.JobEmploymentType
import com.codeiy.im.model.JobFeedbackRequest
import com.codeiy.im.model.JobFeedbackType
import com.codeiy.im.model.JobOpportunity
import com.codeiy.im.model.JobOpportunityDetail
import com.codeiy.im.model.JobProfileParseRequest
import com.codeiy.im.model.JobSearchProfile
import com.codeiy.im.model.JobSearchProfilePreview
import com.codeiy.im.model.JobSearchProfileWrite
import com.codeiy.im.model.JobSeniority
import com.codeiy.im.model.JobWorkMode
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

data class JobFilters(
    val profileId: String? = null,
    val query: String = "",
    val source: IMChannel? = null,
    val workMode: JobWorkMode? = null,
    val employmentType: JobEmploymentType? = null,
    val seniority: JobSeniority? = null,
    val minimumMatchScore: Int = 0,
    val excludeExpired: Boolean = true,
    val excludeExplicitAgeRequirement: Boolean = false,
    val sort: String = "match",
)

class JobDiscoveryViewModel(private val service: RadarApi) : ViewModel() {
    data class UiState(
        val items: List<JobOpportunity> = emptyList(),
        val profiles: List<JobSearchProfile> = emptyList(),
        val filters: JobFilters = JobFilters(),
        val total: Int = 0,
        val loading: Boolean = false,
        val error: String? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state
    private var loadJob: Job? = null

    fun load() {
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            _state.value = _state.value.copy(loading = true, error = null)
            try {
                val profiles = api { service.jobSearchProfiles() }
                val selectedId = _state.value.filters.profileId
                    ?: profiles.firstOrNull { it.isDefault }?.id
                    ?: profiles.firstOrNull()?.id
                val filters = _state.value.filters.copy(profileId = selectedId)
                val page = fetch(filters)
                _state.value = _state.value.copy(
                    items = page.items,
                    profiles = profiles,
                    filters = filters,
                    total = page.total,
                    loading = false,
                )
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _state.value = _state.value.copy(loading = false, error = error.message)
            }
        }
    }

    fun applyFilters(filters: JobFilters) {
        _state.value = _state.value.copy(filters = filters)
        load()
    }

    private suspend fun fetch(filters: JobFilters) = api {
        service.jobs(
            profileId = filters.profileId,
            query = filters.query.trim().ifEmpty { null },
            source = filters.source?.serialValue(),
            workMode = filters.workMode?.serialValue(),
            employmentType = filters.employmentType?.serialValue(),
            seniority = filters.seniority?.serialValue(),
            minimumMatchScore = filters.minimumMatchScore.takeIf { it > 0 },
            ageRequirementPresent = if (filters.excludeExplicitAgeRequirement) false else null,
            excludeExpired = filters.excludeExpired,
            sort = filters.sort,
        )
    }
}

class JobDetailViewModel(
    private val service: RadarApi,
    private val opportunityId: String,
    private val profileId: String?,
) : ViewModel() {
    data class UiState(
        val detail: JobOpportunityDetail? = null,
        val loading: Boolean = false,
        val error: String? = null,
        val feedbackSent: JobFeedbackType? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state

    fun load() = viewModelScope.launch {
        _state.value = _state.value.copy(loading = true, error = null)
        try {
            val detail = api { service.job(opportunityId, profileId) }
            _state.value = _state.value.copy(detail = detail, loading = false)
        } catch (error: Exception) {
            _state.value = _state.value.copy(loading = false, error = error.message)
        }
    }

    fun feedback(type: JobFeedbackType) = viewModelScope.launch {
        try {
            api { service.saveJobFeedback(opportunityId, JobFeedbackRequest(type)) }
            _state.value = _state.value.copy(feedbackSent = type, error = null)
        } catch (error: Exception) {
            _state.value = _state.value.copy(error = error.message)
        }
    }
}

class JobProfilesViewModel(private val service: RadarApi) : ViewModel() {
    data class UiState(
        val profiles: List<JobSearchProfile> = emptyList(),
        val preview: JobSearchProfilePreview? = null,
        val parsing: Boolean = false,
        val saving: Boolean = false,
        val error: String? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state

    fun load() = viewModelScope.launch {
        try {
            _state.value = _state.value.copy(profiles = api { service.jobSearchProfiles() }, error = null)
        } catch (error: Exception) {
            _state.value = _state.value.copy(error = error.message)
        }
    }

    fun parse(text: String) = viewModelScope.launch {
        _state.value = _state.value.copy(parsing = true, preview = null, error = null)
        try {
            val preview = api { service.parseJobSearchProfile(JobProfileParseRequest(text.trim())) }
            _state.value = _state.value.copy(preview = preview, parsing = false)
        } catch (error: Exception) {
            _state.value = _state.value.copy(parsing = false, error = error.message)
        }
    }

    fun savePreview() = viewModelScope.launch {
        val preview = _state.value.preview ?: return@launch
        _state.value = _state.value.copy(saving = true, error = null)
        try {
            api { service.createJobSearchProfile(preview.toWrite()) }
            _state.value = _state.value.copy(
                profiles = api { service.jobSearchProfiles() },
                preview = null,
                saving = false,
            )
        } catch (error: Exception) {
            _state.value = _state.value.copy(saving = false, error = error.message)
        }
    }

    fun save(profile: JobSearchProfile?, body: JobSearchProfileWrite) = viewModelScope.launch {
        _state.value = _state.value.copy(saving = true, error = null)
        try {
            if (profile == null) {
                api { service.createJobSearchProfile(body) }
            } else {
                api { service.updateJobSearchProfile(profile.id, body) }
            }
            _state.value = _state.value.copy(profiles = api { service.jobSearchProfiles() }, saving = false)
        } catch (error: Exception) {
            _state.value = _state.value.copy(saving = false, error = error.message)
        }
    }

    fun delete(profile: JobSearchProfile) = viewModelScope.launch {
        try {
            api { service.deleteJobSearchProfile(profile.id) }
            _state.value = _state.value.copy(profiles = api { service.jobSearchProfiles() }, error = null)
        } catch (error: Exception) {
            _state.value = _state.value.copy(error = error.message)
        }
    }

    fun discardPreview() {
        _state.value = _state.value.copy(preview = null)
    }
}

private fun IMChannel.serialValue() = when (this) {
    IMChannel.TELEGRAM -> "telegram"
    IMChannel.WECOM -> "wecom"
    IMChannel.UNKNOWN -> "unknown"
}

private fun JobWorkMode.serialValue() = when (this) {
    JobWorkMode.REMOTE -> "remote"
    JobWorkMode.HYBRID -> "hybrid"
    JobWorkMode.ON_SITE -> "on_site"
    JobWorkMode.FLEXIBLE -> "flexible"
    JobWorkMode.UNKNOWN -> "unknown"
}

private fun JobEmploymentType.serialValue() = when (this) {
    JobEmploymentType.FULL_TIME -> "full_time"
    JobEmploymentType.PART_TIME -> "part_time"
    JobEmploymentType.CONTRACT -> "contract"
    JobEmploymentType.INTERNSHIP -> "internship"
    JobEmploymentType.FREELANCE -> "freelance"
    JobEmploymentType.TEMPORARY -> "temporary"
    JobEmploymentType.UNKNOWN -> "unknown"
}

private fun JobSeniority.serialValue() = name.lowercase()
