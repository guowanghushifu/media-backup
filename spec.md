# media-backup 实现说明

## 1. 程序目标

`media-backup` 是一个面向视频文件备份/上传的常驻程序。它从 YAML 配置文件读取多个 `jobs`，每个 job 定义源目录、可选硬链接目录和 rclone 远端目录。程序启动后会：

1. 递归监控每个 `source_dir` 中的视频文件变更。
2. 等待新文件大小稳定，避免上传尚未写完的文件。
3. 如果 `link_dir` 非空且不同于 `source_dir`，将源文件按相对路径硬链接到对应 `link_dir`；如果 `link_dir` 为空或等于 `source_dir`，跳过硬链接，直接把源文件作为上传文件。
4. 为每个待上传文件注册独立上传任务。
5. 通过内部调度器控制并发、重试和同路径文件替换。
6. 调用 `rclone copy <upload_file> <remote_dir>/ ...args` 上传单个文件。
7. 上传成功后，如果任务使用独立 `link_dir`，删除该硬链接文件并清理空的父目录；如果任务直传源文件，则保留源文件，不执行上传后文件清理。
8. 在终端实时展示活动任务、排队数和最近事件；同时写入按天轮转的日志。
9. 可选地在达到最大重试次数后发送 Telegram 最终失败通知。

## 2. 目录和模块结构

核心 Go 代码分布如下：

- `cmd/media-backup/main.go`：命令行入口、配置路径解析、日志初始化、信号处理。
- `internal/config`：配置结构、默认值、规范化和校验。
- `internal/watcher`：文件扫描、稳定性等待、硬链接创建或直传路径选择、上传后清理。
- `internal/queue`：上传任务调度器，负责去重、并发限制、失败重试状态。
- `internal/rclone`：rclone 命令构造、代理环境变量、输出解析、远端路径计算。
- `internal/app`：主服务编排，串联监控、扫描、链接、调度、上传、UI、日志和通知。
- `internal/ui`：终端 Dashboard 渲染、ANSI 刷新、宽字符对齐、终端宽度检测。

## 3. 配置文件

配置示例位于 `configs/config.example.yaml`。主配置结构由 `internal/config/config.go` 中的 `Config` 定义。

### 3.1 顶层字段

- `poll_interval`：等待文件稳定时的轮询间隔。默认 `1s`。
- `stable_duration`：文件大小保持不变多久才认为稳定。默认 `1m`；示例配置使用 `30s`。
- `retry_interval`：上传失败后等待多久再重试。默认 `10m`。
- `max_retry_count`：单个文件连续失败次数上限。默认 `0`，表示无限重试；小于 0 会报错。
- `max_parallel_uploads`：全局最大并发上传数。默认 `5`；调度器中小于等于 0 时会退化为 `1`。
- `extensions`：允许处理的视频扩展名列表。默认 `.mkv`、`.mp4`、`.m2ts`、`.ts`，加载后统一转小写。
- `rclone_args`：附加到 rclone 命令末尾的参数。为空时使用内置默认参数。
- `proxy`：rclone 命令和 Telegram 通知使用的 HTTP/HTTPS 代理配置。
- `telegram`：最终失败通知配置。
- `jobs`：任务列表，至少需要一个 job。

### 3.2 job 字段

每个 job 包含：

- `name`：任务显示名，用于 UI 和日志/事件。
- `source_dir`：源目录，程序递归监控这里的文件。
- `link_dir`：硬链接目录。为空或等于 `source_dir` 时启用直传源文件模式，不创建硬链接；非空且不同于 `source_dir` 时，程序将待上传文件硬链接到这里。
- `rclone_remote`：rclone 远端根目录。

配置校验要求：

- `jobs` 不能为空。
- 每个 job 的 `name`、`source_dir`、`rclone_remote` 都不能为空。
- `link_dir` 可以为空；为空时等价于直传源文件模式。
- `source_dir` 不能重复。
- 非空 `link_dir` 不能重复。
- `link_dir` 等于 `source_dir` 时允许，表示直传源文件模式。
- `link_dir` 位于同一个 job 的 `source_dir` 内部时不允许，除非二者相等。
- 不同 job 之间不允许出现 `source_dir` 和非空 `link_dir` 的交叉嵌套关系，避免一个 job 把另一个 job 的上传缓冲区当作源目录处理。
- 启用代理时，`scheme` 只能是 `http` 或 `https`，且必须配置 `host` 和大于 0 的 `port`。
- 启用 Telegram 时，必须配置非空的 `bot_token` 和 `chat_id`。

## 4. 启动流程

入口在 `cmd/media-backup/main.go`：

1. 解析 `-config` 参数。
2. 根据可执行文件所在目录计算日志目录：`<exe_dir>/logs`。
3. 创建 `DailyLogWriter`，日志文件名格式为 `media-backup-YYYY-MM-DD.log`。
4. 解析配置路径：
   - 如果传入 `-config`，直接使用该路径。
   - 否则查找可执行文件同目录下的 `config.yaml`。
   - 找不到时报错：要求指定 `-config` 或把 `config.yaml` 放在可执行文件旁边。
5. 加载并校验配置。
6. 创建 `app.Service`。
7. 监听 `SIGINT` 和 `SIGTERM`，收到后关闭 stop channel。
8. 通过 `app.App.Run` 将 stop channel 转换为 context 取消，驱动服务退出。

## 5. 日志实现

日志由 `internal/app/logging.go` 实现：

- 日志按天写入 `<exe_dir>/logs/media-backup-YYYY-MM-DD.log`。
- 每次写日志前检查当前日期，日期变化时自动切换到新文件。
- 保留最近 7 天日志，清理更早的匹配日志文件。
- 只清理文件名符合 `media-backup-*.log` 且日期可解析的文件。
- 主要记录配置路径、rclone 原始输出、rclone 命令、watcher 错误、上传/清理/通知失败等。

## 6. Service 总体编排

`internal/app/service.go` 中的 `Service` 是核心编排对象。创建时会初始化：

- `fsnotify.Watcher`：监听文件系统事件。
- `queue.Scheduler`：上传任务调度器。
- `configJobs`：以 `source_dir` 为 key 的配置 job 映射。
- `jobs`：以实际上传文件路径为 key 的运行时任务表。独立 `link_dir` 模式下 key 是硬链接路径；直传源文件模式下 key 是源文件路径。
- `processing`：防止同一个源文件被多个 goroutine 重复处理。
- `retryDue`：记录失败任务下次允许重试的时间。
- `failureCounts`：记录每个上传文件路径的连续失败次数。
- `processSem`：限制前置文件处理并发，当前固定最多 32 个文件同时执行稳定等待、上传路径准备和任务注册。
- `wakeCh`：唤醒调度循环。
- `uiWakeCh`：唤醒 UI 刷新。
- Telegram notifier：仅在配置启用时创建。

如果配置启用了 Telegram 通知，并且同时启用了 HTTP 代理，Telegram notifier 会使用带代理的 HTTP client。

`Service.Run` 的顺序：

1. 创建可取消的 `runCtx`。
2. 启动 UI 循环。UI 会在初始扫描前先进入候补屏幕并渲染一次。
3. 执行 `startupCatchUp`，处理启动时已有文件。
4. 启动 `eventLoop` 监听 fsnotify 事件。
5. 启动 `dispatchLoop` 负责重试释放和上传调度。
6. 等待外层 context 取消。
7. 取消内部 context，等待 goroutine 退出，关闭 watcher。

## 7. 启动补偿扫描

启动补偿由 `startupCatchUp` 实现，用于避免程序重启期间遗漏文件。

对每个配置 job：

1. 判断 job 是否为直传源文件模式：`link_dir` 为空或等于 `source_dir`。
2. 如果不是直传源文件模式，创建 `link_dir`。
3. 递归为 `source_dir` 下所有目录添加 fsnotify watch。
4. 扫描源目录中已存在的视频文件。
5. 独立 `link_dir` 模式下，对扫描到的文件创建硬链接；直传源文件模式下，直接注册源文件。
6. 独立 `link_dir` 模式下，扫描 `link_dir` 中所有允许扩展名的文件。
7. 将每个待上传文件注册为独立上传任务，并标记为 dirty/待上传。
8. 如果有待上传文件，写入最近事件：启动扫描发现文件，或链接目录发现待上传文件。

`ScanExistingAndLink` 的稳定性策略：

- 对旧文件：如果文件修改时间早于 `stable_duration`，不再等待。
- 对最近修改的文件：继续调用可取消的稳定等待逻辑，等待大小稳定。
- 如果单个文件持续不稳定超过 `stable_duration + 1m`，启动扫描会跳过该文件并继续扫描后续文件。

因此，启动时已有且看起来已经写完的文件会更快进入上传；刚写入或仍在写入的文件会等待稳定。

## 8. 文件监控和事件处理

事件处理在 `eventLoop` 和 `handleEvent` 中完成。

### 8.1 job 匹配

`findJob` 会用事件路径匹配配置中的 `source_dir`：

- 路径等于某个 `source_dir`，或
- 路径位于某个 `source_dir` 之下。

未匹配到 job 的事件会被忽略。

### 8.2 目录事件

如果事件路径是目录，并且事件类型包含 `Create` 或 `Rename`：

1. 对该目录递归添加 watch。
2. 递归扫描该目录下允许扩展名的视频文件。
3. 对每个文件异步调用 `processFile`。

### 8.3 文件事件

如果事件类型包含 `Create`、`Write` 或 `Rename`，并且扩展名在允许列表中，程序会异步处理该文件。

`processFile` 会先用 `processing` map 去重：同一路径已经在处理时，新事件直接跳过。处理完成后释放该路径。

## 9. 文件稳定检测

`watcher.WaitStable` 负责判断文件是否写完：

1. 先读取当前文件大小。
2. 如果 `stable_duration <= 0`，立即返回成功。
3. 按 `poll_interval` 周期检查文件大小。
4. 只要大小变化，就重置稳定开始时间。
5. 大小持续不变达到 `stable_duration` 后返回成功。
6. 如果总等待时间超过 `stable_duration + 1m`，返回稳定等待超时错误。
7. 如果服务 context 被取消，立即返回 context 错误。
8. 检查过程中 `os.Stat` 失败会返回错误。

如果 `poll_interval <= 0`，函数内部会使用 `100ms` 作为兜底值。运行期处理文件时，稳定等待超时只记录日志并释放该路径，后续文件继续变化产生 fsnotify 事件时会再次触发处理。

## 10. 上传路径准备

上传路径准备逻辑在 `watcher.LinkFile` 或等价 helper 中实现：

1. 如果 `linkDir` 为空或等于 `sourceDir`，不调用 `os.Link`，直接返回源文件路径作为 `uploadPath`，并标记为直传源文件模式。
2. 否则计算 `sourceFile` 相对于 `sourceDir` 的相对路径。
3. 将相对路径拼到 `linkDir` 下，得到 `linkPath`。
4. 创建 `linkPath` 的父目录。
5. 调用 `os.Link(sourceFile, linkPath)` 创建硬链接，并返回 `linkPath` 作为 `uploadPath`。

独立 `link_dir` 模式下，如果目标路径已存在：

- 若目标与源文件是同一个 inode，认为已经链接成功，直接返回。
- 若目标存在但不是同一个文件，认为同路径源文件可能被替换，调用 `replaceHardLink`：
  - 先创建指向新源文件的临时硬链接。
  - 再用 `os.Rename` 原子替换旧的 `linkPath`。
  - 最多尝试 10 次不同临时文件名。

这样可以支持“同一路径的视频文件被新 inode 替换”的场景，并尽量避免上传目录中出现半成品状态。

直传源文件模式下，上传成功后不能删除 `uploadPath`，因为它就是源文件。

## 11. 源目录扫描

`watcher.ScanExistingAndLink` 和 `watcher.ScanAndLink` 都基于 `scanAndLink`：

- 使用 `filepath.WalkDir` 递归遍历 `source_dir`。
- 只处理普通文件，目录继续递归。
- 配置层不允许 `link_dir` 嵌套在 `source_dir` 内部，除非二者相等。扫描层仍应把非直传模式下的 `link_dir` 作为防御性跳过目录，避免异常配置或历史目录导致自递归处理。
- 扩展名比较不区分大小写。
- 处理文件前可执行稳定性等待。
- 独立 `link_dir` 模式下，对匹配文件调用 `LinkFile`；直传源文件模式下，直接返回源文件作为待上传文件。

`ScanLinkedFiles` 则递归扫描独立 `link_dir`，返回所有允许扩展名的待上传文件路径。直传源文件模式不扫描 `link_dir`。

## 12. 远端目录映射

远端目录由 `rclone.BuildRemoteDir` 计算：

1. 计算 `sourceFile` 相对于 `sourceDir` 的相对路径。
2. 如果源文件不在源目录内，返回错误。
3. 取相对路径的父目录作为远端子目录。
4. 将该子目录拼到 `rclone_remote` 后面。
5. 统一使用 `/` 作为远端路径分隔符。
6. 除 bare remote root 形式如 `gd1:` 外，确保目录以 `/` 结尾。

示例：

- `source_dir`: `/data/in`
- `rclone_remote`: `gd1:/sync/Movie`
- `sourceFile`: `/data/in/A/B/movie.mkv`
- `remoteDir`: `gd1:/sync/Movie/A/B/`

上传时调用的是单文件复制：

```text
rclone copy <upload_file> <remoteDir>/ <rclone_args...>
```

注意：独立 `link_dir` 模式下，程序上传的是 `link_dir` 下的单个硬链接文件；直传源文件模式下，程序上传的是源文件本身。两种模式的远端目录都根据原始 `source_dir` 的相对路径计算。

## 13. 任务模型

运行时上传任务使用 `jobRuntime` 表示，关键字段包括：

- `cfg`：所属配置 job。
- `key`：任务 key，当前实现为实际上传文件路径。
- `sourcePath`：源文件路径。
- `uploadPath`：实际传给 rclone 的本地文件路径。独立 `link_dir` 模式下是硬链接文件路径；直传源文件模式下是源文件路径。
- `linkPath`：硬链接文件路径，仅独立 `link_dir` 模式有意义；直传源文件模式下可与 `sourcePath`/`uploadPath` 相同或为空，但不能触发上传后删除。
- `remoteDir`：该文件应该上传到的 rclone 远端目录。
- `summary`：UI 展示的上传摘要。
- `active`：是否正在上传。
- `cancel`：用于取消当前上传的 context cancel cause。

任务表 `Service.jobs` 使用 `uploadPath` 作为 key。这意味着同一个 job 下的不同文件是独立任务，兄弟文件可并发上传；同一路径的文件更新会复用同一个 key，并触发替换逻辑。

## 14. 注册任务和同路径更新

文件稳定并准备上传路径后，`linkAndQueueTask` 会：

1. 独立 `link_dir` 模式下创建或更新硬链接；直传源文件模式下跳过硬链接并使用源文件路径。
2. 判断同路径任务是否已有失败计数，必要时准备重置。
3. 调用 `registerTaskLocked` 注册运行时任务。
4. 将任务标记为 dirty。
5. 唤醒调度器。

`registerTaskLocked` 会处理三种重要状态：

- `wasQueued`：该 key 已经在待上传队列中。
- `clearedRetryWait`：该 key 正在等待重试，新文件到来时清除等待并重新排队。
- `replacedRunning`：该 key 正在上传，新文件到来时取消旧上传并让新任务重新排队。

当同路径文件在上传中被更新时：

1. 创建新的 `jobRuntime` 替换 `jobs[uploadPath]`。
2. 删除旧 key 的失败计数。
3. 调用旧任务的 cancel 函数，cause 为 `errUploadSuperseded`。
4. 旧上传结束后进入 `finishUploadSuperseded`，不会清理上传文件，也不会按普通失败重试。
5. 调度器通过 `Finish(job.key, true)` 将该 key 重新排队。

当同路径文件在重试等待中被更新时：

1. 删除 `retryDue`。
2. 删除失败计数。
3. 调用 `scheduler.RetryJob` 将等待状态恢复到可排队状态。
4. 再执行 `MarkDirty` 确保任务待上传。

## 15. 调度器实现

调度器位于 `internal/queue/scheduler.go`，负责纯状态管理，不直接启动 goroutine。

### 15.1 状态字段

每个任务 key 对应一个 `jobState`：

- `queued`：是否在队列中等待开始。
- `running`：是否正在运行。
- `dirty`：运行期间是否有新变更，需要结束后重新上传。
- `pendingRetry`：是否处于失败后的重试等待。

调度器自身还维护：

- `maxParallel`：最大并发数。
- `active`：当前运行数。
- `order`：待运行 key 的稳定顺序队列。
- `retries`：重试集合。

### 15.2 MarkDirty

`MarkDirty(job)` 表示该任务有新内容需要上传：

- 设置 `dirty = true`。
- 如果任务正在等待重试，则不立即入队。
- 如果任务既未 queued 也未 running，则加入 `order` 队列。
- 对同一 key 多次调用会去重。

### 15.3 Ready / TryStart

`Ready()` 返回当前 queued 且未 running 的 key，保持 `order` 顺序。

`TryStart(job)` 尝试启动一个任务：

- 如果达到 `maxParallel`，返回 false。
- 如果任务已经 running、未 queued、或 pendingRetry，返回 false。
- 成功时：
  - `queued = false`
  - `running = true`
  - `dirty = false`
  - `active++`
  - 从 `order` 中移除

### 15.4 Finish

`Finish(job, dirty)` 表示任务运行结束：

- 如果任务 running，减少 `active`。
- 清除 running 和 pendingRetry。
- 如果参数 `dirty` 为 true，或运行期间 state.dirty 被重新设置，则重新入队。

这用于处理上传过程中同路径文件更新：旧上传被取消后结束，但 dirty=true 会让新任务继续上传。

### 15.5 FinishFailed / RetryJob

`FinishFailed(job)` 表示任务上传失败并进入重试等待：

- 减少 active。
- 清除 running 和 queued。
- 设置 `pendingRetry = true`。
- 从 `order` 移除。
- 放入 `retries` 集合。

`RetryJob(job)` 表示重试时间已到或外部主动释放：

- 必须存在于 `retries` 中才会生效。
- 清除 `pendingRetry`。
- 如果未 queued 且未 running，则重新加入 `order`。

### 15.6 Forget

`Forget(job)` 只会删除终态任务：

- 任务不能 queued。
- 任务不能 running。
- 任务不能 pendingRetry。

上传成功且没有新 dirty 状态时，Service 会调用 `Forget` 并从运行时任务表删除任务。

## 16. 调度循环

`Service.dispatchLoop` 每 500ms tick 一次，同时也可被 `wakeCh` 立即唤醒。每轮执行：

1. `releaseRetries()`：释放到期的重试任务。
2. `startReadyUploads(ctx)`：启动可运行的上传任务。

### 16.1 释放重试

`releaseRetries` 会检查 `retryDue`：

- 当前时间达到或超过 due time 的 key 被取出。
- 调用 `scheduler.RetryJob(key)`。
- 如果成功，记录“到达重试时间，重新排队”事件。
- 唤醒调度器和 UI。

### 16.2 启动上传

`startReadyUploads` 遍历 `scheduler.Ready()`：

1. 对每个 key 调用 `TryStart`，由调度器保证并发限制。
2. 调用 `activateTaskForUpload` 标记 `active=true`，设置 summary 为“等待 rclone 输出”，并创建可取消的 upload context。
3. 记录“调度开始上传”事件。
4. 通过 `startUpload` 启动 goroutine 执行 `runUpload`。

如果调度器中存在 key 但运行时任务表找不到对应任务，Service 会结束并遗忘该 key，避免队列卡住。

## 17. 上传执行

上传由 `runUpload` 和 `copyWithRclone` 完成。

### 17.1 rclone 调用

`copyWithRclone` 创建 `rclone.CommandExecutor`，再用 `rclone.Runner.CopyFile` 执行：

```text
rclone copy <uploadPath> <remoteDirWithTrailingSlash> <rclone_args...>
```

特点：

- 默认二进制名是 `rclone`。
- stdout 和 stderr 都会被逐行扫描。
- 每一行输出都写入日志。
- 统计行会更新 UI 的任务摘要。
- “Copied (...)” 事件会加入最近事件。
- 如果启用代理，会给 rclone 子进程附加 `HTTP_PROXY`、`HTTPS_PROXY`、`http_proxy`、`https_proxy` 环境变量。

### 17.2 成功路径

rclone 返回成功后：

1. 如果 context cause 是 `errUploadSuperseded`，按“被新文件替换”处理。
2. 否则将任务标记为 inactive，summary 设为“上传完成”。
3. 如果任务是独立 `link_dir` 模式，调用 `CleanupLinkedFile(linkDir, uploadPath)` 删除硬链接文件。
4. 独立 `link_dir` 模式下，删除后向上清理空父目录，但不会删除 `link_dir` 根目录。
5. 如果任务是直传源文件模式，跳过 `CleanupLinkedFile`，必须保留源文件。
6. 清理成功或跳过清理后，清空失败计数。
7. 调度器 `Finish(key, false)`。
8. 如果任务已无 queued/running/retry 状态，调用 `Forget` 并从 `jobs` 删除。
9. 记录“上传完成，任务清空”或“上传完成，任务保留”。

独立 `link_dir` 模式下，如果清理时发现硬链接文件已经不存在，程序记录日志，但仍视为上传完成。直传源文件模式下不应因为上传成功而删除或移动源文件。

### 17.3 失败路径

rclone 返回错误时：

1. 如果 context cause 是 `errUploadSuperseded`，不算失败，走替换完成逻辑。
2. 否则调用 `finishUploadFailure`。

普通失败处理：

1. 增加该 key 的连续失败计数。
2. 将任务标记为 inactive，summary 记录错误。
3. 如果当前运行时任务仍是该 job，并且未达到最大重试次数：
   - 设置 `retryDue[key] = now + retry_interval`。
   - 调用 `scheduler.FinishFailed(key)`。
   - 记录“上传失败，进入重试等待”。
4. 如果达到最大重试次数：
   - 调用 `scheduler.Finish(key, false)`。
   - 记录“上传失败，达到最大重试次数，停止重试”。
   - 清理该 key 的内存状态，包括运行时任务、失败计数、重试时间和调度器终态状态。
   - 如果启用 Telegram，发送最终失败通知。
5. 失败时不会删除上传文件。独立 `link_dir` 模式下，硬链接文件保留以便重试；直传源文件模式下，源文件本来就必须保留。达到重试上限后，文件仍保留，等待新事件或重启扫描重新注册。

`max_retry_count = 0` 表示无限重试。对于大于 0 的值，当前实现中 `failures < max_retry_count` 时继续重试，达到该次数时停止。

## 18. 上传后清理

`watcher.CleanupLinkedFile` 只允许删除独立 `link_dir` 内部的硬链接文件：

1. 对 `linkDir` 和 `linkFile` 做 clean。
2. 计算相对路径。
3. 如果目标不是 `link_dir` 内的子路径，返回错误，不删除任何文件。
4. 删除硬链接文件。
5. 从该文件父目录开始，逐级删除空目录。
6. 遇到非空目录或到达 `link_dir` 根目录时停止。

这个逻辑保证上传完成只清理上传缓冲区中的硬链接，不会删除源文件，也不会删除 `link_dir` 根目录。直传源文件模式必须完全跳过该清理逻辑。

## 19. rclone 输出解析

解析逻辑在 `internal/rclone/parser.go`。

### 19.1 INFO payload

程序只关注包含 `INFO  :` 的输出行。`ParseInfoPayload` 会提取 marker 后的文本。

### 19.2 统计行

`ParseStats` 判断 payload 同时包含：

- ` / `
- `ETA`

满足条件时认为是 rclone stats 行，用于更新 UI summary。

UI 会把 summary 按 `, ` 拆分，并从中提取：

- 进度：第 2 段。
- 速度：第 3 段。
- ETA：第 4 段，会将 `ETA 1m2s` 这类 Go duration 转换为中文“预计 MM:SS”或“预计 HH:MM:SS”。

### 19.3 完成事件

`ParseEvent` 判断 payload 是否包含 `Copied (`。匹配到时会作为最近事件展示。

## 20. 代理支持

代理会作用于 rclone 子进程和 Telegram 通知请求。

启用 `proxy.enabled` 后，程序会构造代理 URL：

```text
<scheme>://[username[:password]@]<host>:<port>
```

并设置四个环境变量：

- `HTTP_PROXY`
- `HTTPS_PROXY`
- `http_proxy`
- `https_proxy`

用户名和密码会通过 `net/url` 正确转义。

Telegram 通知不会使用环境变量，而是在 HTTP client 的 `Transport.Proxy` 中直接配置同一个代理 URL。

## 21. Telegram 通知

Telegram 通知在 `internal/app/telegram.go` 中实现。

启用条件：

- `telegram.enabled = true`
- 配置了 `bot_token`
- 配置了 `chat_id`

触发时机：

- 单个文件上传失败达到 `max_retry_count`，并停止重试时。

请求行为：

- 使用 `POST https://api.telegram.org/bot<token>/sendMessage`。
- 请求体是 JSON，包含 `chat_id` 和文本消息。
- HTTP client timeout 为 5 秒。
- 如果 `proxy.enabled = true`，HTTP client 会通过配置的代理发送请求。
- 非 2xx 响应视为发送失败。
- 发送失败只写日志，不影响主流程。

通知内容包含 job 名称、文件路径、失败次数和最后错误。

## 22. 终端 UI

UI 由 `internal/ui` 和 `Service.uiLoop` 实现。

### 22.1 刷新机制

- 启动时进入 alternate screen，并隐藏光标。
- 退出时显示光标并离开 alternate screen。
- 每秒 tick 一次。
- 有任务状态或事件变化时通过 `uiWakeCh` 立即刷新。
- 如果状态无变化，也会每 3 秒 keepalive 刷新一次。
- 如果终端宽度变化，执行 full refresh。

### 22.2 Dashboard 内容

Dashboard 包含三个面板：

1. 系统状态：空闲/排队中/运行中、活动数、最大并发、排队数、更新时间。
2. 活动任务：展示正在上传文件的 basename、进度、速度、预计时间、状态。
3. 最近事件：最多保留 10 条，最新事件显示在最上方。

### 22.3 宽度和中文对齐

UI 对中日韩宽字符按 2 列宽处理，对组合字符按 0 列宽处理。文本超过列宽时用 `...` 截断，保证中文环境下表格尽量对齐。

## 23. 最近事件

Service 内部维护 `recentEvents`：

- 最多保存 10 条。
- 新事件追加到尾部。
- UI 快照时倒序返回，因此最新事件展示在前。
- 事件产生后会唤醒 UI。

常见事件包括：

- 启动扫描发现文件。
- 链接目录发现待上传文件。
- 检测到新文件。
- 检测到同路径文件更新并取消旧上传。
- 检测到同路径文件更新并清除重试等待。
- 调度开始上传。
- 到达重试时间，重新排队。
- 上传失败，进入重试等待。
- 上传失败，达到最大重试次数，停止重试。
- 上传完成，任务清空/保留。
- rclone 输出的 `Copied (...)` 事件。

## 24. 并发与一致性设计

程序使用多层锁和 channel 保证并发安全：

- `Service.mu`：保护运行时任务表、processing、recentEvents、retryDue、failureCounts 等共享状态。
- `Service.completionMu`：串行化任务注册、上传完成、失败和替换相关的关键路径，避免同路径更新和旧上传完成交错导致误清理。
- `Service.processSem`：限制最多 32 个文件同时执行前置处理，避免大量不同文件事件同时进入稳定等待和上传路径准备阶段。
- `queue.Scheduler.mu`：保护调度器内部状态。
- `processing`：避免同一个源路径被多个事件同时处理。
- `wakeCh` 和 `uiWakeCh` 都是容量 1 的 channel，发送时非阻塞；多个唤醒可合并，避免阻塞业务 goroutine。

特别重要的是同路径替换场景：

- 如果同一路径文件在上传中被新文件替换，旧上传会被 cancel cause 标记为 superseded。
- 旧上传结束后不会删除上传文件。独立 `link_dir` 模式下，硬链接路径可能已经指向新文件；直传源文件模式下，上传路径就是源文件，任何情况下都不能因 superseded 清理而删除。
- 调度器重新排队同一个 key，确保上传最新文件。

## 25. 退出行为

程序收到 `SIGINT` 或 `SIGTERM` 后：

1. 关闭 stop channel。
2. `app.App.Run` 取消 context。
3. `Service.Run` 取消内部 `runCtx`。
4. UI、事件循环、调度循环根据 context 退出。
5. `Service.Close` 关闭 fsnotify watcher。
6. 已启动的 rclone 子进程使用 `exec.CommandContext`，context 取消时会被终止。

## 26. 构建和安装脚本概况

除核心 Go 代码外，仓库还包含：

- `build.sh`：本地构建脚本。
- `ci_build.sh`：CI 构建脚本，测试覆盖了版本参数和归档产物内容。
- `install-systemd-service.sh`：systemd 安装、卸载、重启脚本，测试覆盖了默认安装、架构选择、已安装检测、卸载、延迟重启等场景。

这些脚本不是运行时 Go 代码的一部分，但配合二进制部署使用。

## 27. 端到端流程示例

假设配置如下：

```yaml
jobs:
  - name: MOVIE
    source_dir: /dld/upload/Movie-2025
    link_dir: /dld/gd_upload/Movie-2025
    rclone_remote: gd1:/sync/Movie/Movie-2025
```

当 `/dld/upload/Movie-2025/A/movie.mkv` 写入完成后：

1. fsnotify 捕获 `Create` 或 `Write` 事件。
2. `findJob` 判断该文件属于 `MOVIE` job。
3. `processFile` 等待文件大小稳定。
4. `LinkFile` 创建硬链接：
   - 源文件：`/dld/upload/Movie-2025/A/movie.mkv`
   - 硬链接：`/dld/gd_upload/Movie-2025/A/movie.mkv`
5. `BuildRemoteDir` 计算远端目录：`gd1:/sync/Movie/Movie-2025/A/`。
6. Service 注册任务 key：`/dld/gd_upload/Movie-2025/A/movie.mkv`。
7. Scheduler 标记任务 dirty 并排队。
8. 调度循环在并发额度允许时启动上传。
9. 执行命令：

```text
rclone copy /dld/gd_upload/Movie-2025/A/movie.mkv gd1:/sync/Movie/Movie-2025/A/ <rclone_args...>
```

10. rclone 输出进度时，UI 活动任务行更新。
11. rclone 成功返回后，删除硬链接 `/dld/gd_upload/Movie-2025/A/movie.mkv`。
12. 如果 `A` 目录为空，也会删除 `/dld/gd_upload/Movie-2025/A`。
13. 源文件 `/dld/upload/Movie-2025/A/movie.mkv` 不会被删除。
14. 任务从调度器和运行时任务表清空，UI 记录完成事件。

如果该 job 配置为直传源文件模式：

```yaml
jobs:
  - name: MOVIE
    source_dir: /dld/upload/Movie-2025
    link_dir: ""
    rclone_remote: gd1:/sync/Movie/Movie-2025
```

或：

```yaml
jobs:
  - name: MOVIE
    source_dir: /dld/upload/Movie-2025
    link_dir: /dld/upload/Movie-2025
    rclone_remote: gd1:/sync/Movie/Movie-2025
```

则第 4 步不会创建硬链接，任务 key 和上传文件都是 `/dld/upload/Movie-2025/A/movie.mkv`。rclone 成功返回后不调用 `CleanupLinkedFile`，源文件必须继续保留。

## 28. 当前实现的边界和注意事项

- 监控依赖 fsnotify，对已存在目录会在启动时递归添加 watch；新建目录会在目录事件中递归添加 watch。
- 文件稳定性只通过文件大小判断，不校验文件内容是否仍在变化但大小不变。
- 独立 `link_dir` 模式要求 `source_dir` 和 `link_dir` 所在文件系统支持 hard link；跨文件系统硬链接会失败。直传源文件模式不需要 hard link。
- 调度 key 是实际上传文件路径，因此同一路径更新会替换任务，不同路径文件互不影响。
- 上传失败不会删除上传文件，保证可以重试。
- 达到最大重试次数后任务会停止自动重试，并释放内存状态，但上传文件仍保留。独立 `link_dir` 模式下保留在 `link_dir`；直传源文件模式下保留源文件。
- 程序重启后，独立 `link_dir` 模式会扫描 `link_dir`，这些遗留硬链接会再次注册为待上传任务；直传源文件模式会通过源目录扫描重新注册源文件。
- `link_dir` 为空或等于 `source_dir` 时是合法的直传源文件模式；`link_dir` 位于 `source_dir` 内部但不相等、不同行 job 的 `source_dir` 与非空 `link_dir` 交叉嵌套，都是非法配置。
- Telegram 通知只在达到最大重试次数时发送；普通重试失败不发送。
- rclone 输出解析依赖 `INFO  :`、`ETA`、`Copied (` 等文本特征，rclone 输出格式变化可能影响 UI 展示，但不影响上传命令本身。
- 代理配置会同时用于 rclone 子进程和 Telegram HTTP client；其中 rclone 通过环境变量使用代理，Telegram 通过 HTTP transport 使用代理。
