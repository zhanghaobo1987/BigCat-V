# app/libs — gomobile 产物放置处

把 `bigcatv.aar` 放到本目录：

```
mobile/android/app/libs/bigcatv.aar
```

然后取消 `app/build.gradle` 中这一行的注释：

```gradle
// implementation(name: 'bigcatv', ext: 'aar')
```

Kotlin 侧无需任何改动：`core/CoreBridge.kt` 用反射调用
`mobile.Mobile` 的 `Start / Stop / Version` 静态方法，aar 就位后重新构建即生效；
缺失时会在连接时给出明确提示（`CoreBridge.CoreMissingException`）。

aar 由 `core/mobile/` 以以下命令生成（另一个 agent 负责）：

```bash
gomobile bind -target=android -o mobile/android/app/libs/bigcatv.aar ./core/mobile
```

要求：Go 包 `mobile` 导出 `Start(listenAddr, workDir string) error`、
`Stop()`、`Version() string`。
