package com.codeiy.im.core.network

import com.codeiy.im.model.RadarJson
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.HttpException
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.io.IOException

/** 与 iOS APIError 对齐的错误类型：401 触发会话失效，其余展示后端 detail。 */
sealed class ApiError(message: String) : Exception(message) {
    class Unauthorized : ApiError("登录已过期，请重新登录")
    class Server(val status: Int, message: String) : ApiError(message)
    class Network(cause: IOException) : ApiError("网络不可用：${cause.message ?: "连接失败"}")
}

/**
 * 唯一 HTTP 边界（蓝图约束，等同 iOS `Core/Network`）：
 * 所有后端调用都经 [api] 包装，统一注入 Bearer token 与错误映射。
 */
class ApiClient(private val tokenProvider: () -> String?) {

    val service: RadarApi by lazy {
        val client = OkHttpClient.Builder()
            .addInterceptor { chain ->
                val token = tokenProvider()
                val request = if (token != null) {
                    chain.request().newBuilder().header("Authorization", "Bearer $token").build()
                } else {
                    chain.request()
                }
                chain.proceed(request)
            }
            .build()
        Retrofit.Builder()
            .baseUrl("$ApiBaseUrl/")
            .client(client)
            .addConverterFactory(RadarJson.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(RadarApi::class.java)
    }
}

/** 统一错误映射：HttpException → ApiError（解析 FastAPI `{"detail": ...}`）。 */
suspend fun <T> api(block: suspend () -> T): T = try {
    block()
} catch (e: HttpException) {
    if (e.code() == 401) throw ApiError.Unauthorized()
    throw ApiError.Server(e.code(), e.detailMessage())
} catch (e: IOException) {
    throw ApiError.Network(e)
}

private fun HttpException.detailMessage(): String {
    val body = runCatching { response()?.errorBody()?.string() }.getOrNull()
    val detail = body?.let {
        runCatching {
            ((RadarJson.parseToJsonElement(it) as? JsonObject)?.get("detail") as? JsonPrimitive)?.content
        }.getOrNull()
    }
    return detail ?: "请求失败（HTTP ${code()}）"
}
