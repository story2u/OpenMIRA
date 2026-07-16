package com.codeiy.im

import com.codeiy.im.model.JobOpportunityDetail
import com.codeiy.im.model.JobSearchProfilePreview
import com.codeiy.im.model.JobWorkMode
import com.codeiy.im.model.RadarJson
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class JobDiscoveryModelsTest {
    @Test
    fun decodesJobDetailAndPreservesUnknownConstraints() {
        val json = """
            {
              "opportunityId": "job-1",
              "jobTitle": "Backend Engineer",
              "workMode": "remote",
              "postedAt": "2026-07-16T08:00:00Z",
              "rawExcerpt": "Remote backend role",
              "match": {
                "eligibility": "unknown",
                "matchScore": 72,
                "unknownConstraints": ["签证支持未说明"]
              }
            }
        """.trimIndent()

        val detail = RadarJson.decodeFromString<JobOpportunityDetail>(json)

        assertEquals(JobWorkMode.REMOTE, detail.workMode)
        assertEquals(72, detail.match?.matchScore)
        assertEquals(listOf("签证支持未说明"), detail.match?.unknownConstraints)
    }

    @Test
    fun unknownFutureEnumDegradesSafely() {
        val detail = RadarJson.decodeFromString<JobOpportunityDetail>(
            """{"opportunityId":"job-1","jobTitle":"Role","workMode":"distributed_async","rawExcerpt":"Role"}""",
        )

        assertEquals(JobWorkMode.UNKNOWN, detail.workMode)
    }

    @Test
    fun parsedProfileAlwaysRequiresExplicitConfirmation() {
        val preview = RadarJson.decodeFromString<JobSearchProfilePreview>(
            """{"name":"远程后端","targetRoles":["Python Backend"],"requiresConfirmation":true}""",
        )

        assertTrue(preview.requiresConfirmation)
        assertEquals("Python Backend", preview.targetRoles.single())
    }
}
