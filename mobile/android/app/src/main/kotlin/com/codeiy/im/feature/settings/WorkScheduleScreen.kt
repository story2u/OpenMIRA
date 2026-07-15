package com.codeiy.im.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.codeiy.im.model.WorkSchedule
import com.codeiy.im.model.WorkScheduleSlotDTO
import com.codeiy.im.ui.theme.AppColors

private data class DayEntry(val enabled: Boolean, val start: String, val end: String)

private val WEEKDAYS = listOf(1 to "周一", 2 to "周二", 3 to "周三", 4 to "周四", 5 to "周五", 6 to "周六", 7 to "周日")
private val TIMEZONES = listOf("Asia/Shanghai", "Asia/Hong_Kong", "Asia/Tokyo", "Asia/Singapore", "America/Los_Angeles", "America/New_York", "Europe/London", "UTC")
private val HOURS = (8..22).map { "%02d:00".format(it) }

/** 工作时间：一周 7 行，每天开关 + 时段；自动接待需另行授权。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorkScheduleScreen(model: SettingsViewModel, schedule: WorkSchedule, onBack: () -> Unit) {
    val days = remember { mutableStateMapOf<Int, DayEntry>().apply { putAll(decode(schedule.slots)) } }
    var timezone by remember { mutableStateOf(schedule.timezone) }
    var autoReplyOutsideHours by remember { mutableStateOf(schedule.autoReplyOutsideHours) }
    var tzMenu by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }

    val weeklyHours = days.values.filter { it.enabled }.sumOf { hoursBetween(it.start, it.end) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("工作时间") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
                actions = {
                    TextButton(onClick = {
                        error = null
                        model.saveWorkSchedule(timezone, encode(days), autoReplyOutsideHours) {
                            error = it
                        }
                    }) { Text("保存") }
                },
            )
        },
    ) { padding ->
        LazyColumn(Modifier.padding(padding).fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("时区", Modifier.weight(1f))
                    Box {
                        TextButton(onClick = { tzMenu = true }) { Text(timezone) }
                        DropdownMenu(expanded = tzMenu, onDismissRequest = { tzMenu = false }) {
                            TIMEZONES.forEach { tz ->
                                DropdownMenuItem(text = { Text(tz) }, onClick = { timezone = tz; tzMenu = false })
                            }
                        }
                    }
                }
                Text("日夜模式切换依据所选时区判断", style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
            }
            item {
                Row {
                    Text("人工审核时段", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                    Text("本周共 $weeklyHours 小时", style = MaterialTheme.typography.labelSmall, color = AppColors.muted)
                }
            }
            items(WEEKDAYS.size) { index ->
                val (weekday, name) = WEEKDAYS[index]
                val entry = days[weekday] ?: DayEntry(false, "09:00", "18:00")
                Column {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(name, Modifier.weight(1f))
                        Switch(checked = entry.enabled, onCheckedChange = { days[weekday] = entry.copy(enabled = it) })
                    }
                    if (entry.enabled) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            hourPicker(entry.start) { days[weekday] = entry.copy(start = it) }
                            Text("–", color = AppColors.muted)
                            hourPicker(entry.end) { days[weekday] = entry.copy(end = it) }
                        }
                    }
                    HorizontalDivider(Modifier.padding(top = 4.dp))
                }
            }
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("非工作时间 AI 安全接待")
                        Text(
                            "仅对已单独授权的 Telegram Business 私聊生效，发送前仍需通过 Agent 风险检查。",
                            style = MaterialTheme.typography.labelSmall,
                            color = AppColors.muted,
                        )
                    }
                    Switch(
                        checked = autoReplyOutsideHours,
                        onCheckedChange = { autoReplyOutsideHours = it },
                    )
                }
            }
            item {
                TextButton(onClick = { for (d in 1..5) days[d] = DayEntry(true, "09:00", "18:00") }) { Text("工作日统一设为 09:00–18:00") }
                TextButton(onClick = { for (d in 1..7) days[d] = (days[d] ?: DayEntry(false, "09:00", "18:00")).copy(enabled = false) }) {
                    Text("全部清空", color = AppColors.destructive)
                }
                error?.let { Text(it, color = AppColors.destructive, style = MaterialTheme.typography.labelSmall) }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun hourPicker(value: String, onSelect: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        TextButton(onClick = { open = true }) { Text(value) }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            HOURS.forEach { hour ->
                DropdownMenuItem(text = { Text(hour) }, onClick = { onSelect(hour); open = false })
            }
        }
    }
}

private fun decode(slots: List<WorkScheduleSlotDTO>): Map<Int, DayEntry> {
    val result = (1..7).associateWith { DayEntry(false, "09:00", "18:00") }.toMutableMap()
    slots.forEach { result[it.weekday] = DayEntry(true, it.start, it.end) }
    return result
}

private fun encode(days: Map<Int, DayEntry>): List<WorkScheduleSlotDTO> =
    (1..7).mapNotNull { weekday ->
        val entry = days[weekday] ?: return@mapNotNull null
        if (entry.enabled && entry.start < entry.end) WorkScheduleSlotDTO(weekday, entry.start, entry.end) else null
    }

private fun hoursBetween(start: String, end: String): Int =
    (end.substring(0, 2).toIntOrNull() ?: 0) - (start.substring(0, 2).toIntOrNull() ?: 0)
