package com.codeiy.im.ui

import java.time.Duration
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException

/** ISO8601（含小数秒）→ 相对时间；解析失败回退原串。对齐 iOS 的相对时间展示。 */
fun relativeTime(iso: String): String {
    val time = parseIso(iso) ?: return iso
    val seconds = Duration.between(time.toInstant(), OffsetDateTime.now().toInstant()).seconds
    return when {
        seconds < 60 -> "刚刚"
        seconds < 3600 -> "${seconds / 60} 分钟前"
        seconds < 86_400 -> "${seconds / 3600} 小时前"
        else -> "${seconds / 86_400} 天前"
    }
}

/** ISO8601 → "MM-dd HH:mm"；解析失败回退原串。 */
fun shortDateTime(iso: String): String {
    val time = parseIso(iso) ?: return iso
    return time.format(DateTimeFormatter.ofPattern("MM-dd HH:mm"))
}

private fun parseIso(iso: String): OffsetDateTime? = try {
    OffsetDateTime.parse(iso)
} catch (_: DateTimeParseException) {
    null
}
