package com.codeiy.im

import com.codeiy.im.feature.dashboard.DashboardQuery
import com.codeiy.im.model.DashboardResponse
import com.codeiy.im.model.RadarJson
import com.codeiy.im.model.SettingsBundle
import com.codeiy.im.ui.theme.SopStage
import com.codeiy.im.ui.theme.TrustLevel
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 看板/设置 DTO 解码 + DashboardQuery 边界，与 iOS DashboardSettingsTests 对齐。 */
class DashboardSettingsTest {
    @Test
    fun dashboardResponseDecoding() {
        val json = """
        {"items":[],"total":42,"limit":20,"offset":0,"pendingCount":7,
         "attentionItems":[],"keywordOptions":["报价","采购"]}
        """.trimIndent()
        val response = RadarJson.decodeFromString(DashboardResponse.serializer(), json)
        assertEquals(42, response.total)
        assertEquals(7, response.pendingCount)
        assertEquals(listOf("报价", "采购"), response.keywordOptions)
    }

    @Test
    fun settingsBundleDecoding() {
        val json = """
        {
          "detection": {"keywords": ["报价"], "aiSemanticsEnabled": true},
          "workSchedule": {"timezone": "Asia/Shanghai",
            "slots": [{"weekday": 1, "start": "09:00", "end": "18:00"}],
            "autoReplyOutsideHours": true, "isDefault": false},
          "notifications": {"newOpportunityEnabled": true, "aiRepliedEnabled": false,
            "dailyDigestEnabled": false, "urgentOnly": true},
          "capabilities": {"pushAvailable": false, "wecomUserBindingAvailable": false}
        }
        """.trimIndent()
        val bundle = RadarJson.decodeFromString(SettingsBundle.serializer(), json)
        assertEquals(listOf("报价"), bundle.detection.keywords)
        assertEquals(1, bundle.workSchedule.slots.first().weekday)
        assertTrue(bundle.notifications.urgentOnly)
        assertFalse(bundle.capabilities.pushAvailable)
    }

    @Test
    fun trustLevelBoundariesMatchWeb() {
        assertEquals(TrustLevel.TRUSTED, TrustLevel.from(80))
        assertEquals(TrustLevel.UNVERIFIED, TrustLevel.from(79))
        assertEquals(TrustLevel.UNVERIFIED, TrustLevel.from(60))
        assertEquals(TrustLevel.SUSPICIOUS, TrustLevel.from(59))
        assertEquals(TrustLevel.SUSPICIOUS, TrustLevel.from(40))
        assertEquals(TrustLevel.RISKY, TrustLevel.from(39))
    }

    @Test
    fun customRangeValidation() {
        val base = DashboardQuery(timeRange = DashboardQuery.TimeRange.CUSTOM)
        val invalid = base.copy(
            customFrom = java.time.LocalDate.of(2026, 7, 10),
            customTo = java.time.LocalDate.of(2026, 7, 1),
        )
        assertFalse(invalid.customRangeValid)
        val valid = base.copy(
            customFrom = java.time.LocalDate.of(2026, 7, 1),
            customTo = java.time.LocalDate.of(2026, 7, 10),
        )
        assertTrue(valid.customRangeValid)
    }

    @Test
    fun activeAdvancedCount() {
        val query = DashboardQuery(
            source = DashboardQuery.Source.GROUP,
            trustLevels = setOf(TrustLevel.TRUSTED),
        )
        assertEquals(2, query.activeAdvancedCount)
    }

    @Test
    fun todayBoundsUsesTimezone() {
        val query = DashboardQuery(timeRange = DashboardQuery.TimeRange.TODAY, timezoneId = "Asia/Shanghai")
        val (from, to) = query.resolvedBounds()
        assertTrue(from != null)
        assertTrue(to == null)
    }

    @Test
    fun sopStageLabelsFallback() {
        assertEquals("已发现", SopStage.label("detected"))
        assertEquals("some_future_stage", SopStage.label("some_future_stage"))
    }
}
