package com.bigcat.v.ui

import android.content.Context

/** SharedPreferences 轻量设置：API 地址与默认内核。 */
object Prefs {
    private const val FILE = "bigcatv"
    private const val KEY_API_BASE = "api_base"
    private const val KEY_KERNEL = "kernel"

    const val DEFAULT_API_BASE = "http://127.0.0.1:17890"

    private fun prefs(ctx: Context) =
        ctx.getSharedPreferences(FILE, Context.MODE_PRIVATE)

    fun getApiBase(ctx: Context): String =
        prefs(ctx).getString(KEY_API_BASE, DEFAULT_API_BASE) ?: DEFAULT_API_BASE

    fun setApiBase(ctx: Context, value: String) {
        prefs(ctx).edit().putString(KEY_API_BASE, value.trim().trimEnd('/')).apply()
    }

    /** "" = 自动（跟随核心当前选择），否则 "sing-box" / "xray"。 */
    fun getKernel(ctx: Context): String =
        prefs(ctx).getString(KEY_KERNEL, "").orEmpty()

    fun setKernel(ctx: Context, value: String) {
        prefs(ctx).edit().putString(KEY_KERNEL, value).apply()
    }
}
