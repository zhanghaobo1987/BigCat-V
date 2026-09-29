package com.bigcat.v.core

import java.lang.reflect.InvocationTargetException

/**
 * Go 核心（bigcatvd）的 gomobile 集成点。
 *
 * 约定（由 core/mobile/ 的 `gomobile bind -target=android` 产出）：
 *   - 产物：app/libs/bigcatv.aar
 *   - Java 包名：mobile，类名：Mobile
 *   - 静态方法：
 *       Mobile.Start(String listenAddr, String workDir) // 拉起核心并监听，如 "127.0.0.1:17890"
 *       Mobile.Stop()                                  // 停止核心
 *       Mobile.Version() : String                      // 版本号
 *
 * 为“aar 未就位时仍可编译”采用反射调用：aar 放入 app/libs 后无需改任何
 * Kotlin 代码，重新构建即生效。aar 缺失时抛 CoreMissingException（UI 会提示去看 README）。
 */
object CoreBridge {

    private const val CLASS_NAME = "mobile.Mobile"

    class CoreMissingException :
        IllegalStateException("未找到 bigcatv.aar：请按 mobile/android/README.md 生成并放入 app/libs 后重新构建")

    private fun mobileClass(): Class<*> = try {
        Class.forName(CLASS_NAME)
    } catch (_: ClassNotFoundException) {
        throw CoreMissingException()
    }

    private fun invoke(name: String, vararg args: Any?): Any? {
        val cls = mobileClass()
        val types = args.map { it?.javaClass ?: String::class.java }.toTypedArray()
        // Go 的 String 参数在 Java 侧就是 java.lang.String
        val m = cls.methods.firstOrNull { it.name == name && it.parameterTypes.contentEquals(types) }
            ?: throw NoSuchMethodException("$CLASS_NAME.$name(${types.joinToString()}) 不存在")
        return try {
            m.invoke(null, *args)
        } catch (e: InvocationTargetException) {
            // Go 返回的 error 在 Java 侧表现为抛异常，此处解包透出原始信息
            throw RuntimeException(e.targetException?.message ?: e.message, e.targetException)
        }
    }

    /** 拉起 Go 核心。listenAddr 如 "127.0.0.1:17890"，workDir 用 Context.filesDir。 */
    fun start(listenAddr: String, workDir: String) {
        invoke("Start", listenAddr, workDir)
    }

    /** 停止 Go 核心。 */
    fun stop() {
        invoke("Stop")
    }

    /** 核心版本号；aar 缺失时返回 "unknown"。 */
    fun version(): String = try {
        invoke("Version") as? String ?: "unknown"
    } catch (_: Exception) {
        "unknown"
    }

    /** aar 是否已接入。 */
    fun isAvailable(): Boolean = try {
        mobileClass()
        true
    } catch (_: Exception) {
        false
    }
}
