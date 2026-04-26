# media-backup 使用说明

## 使用前准备

需要先安装并配置好：

- `rclone`
- Go 构建环境
- Linux 运行环境，支持 `amd64` 或 `arm64`

先用 `rclone config` 配好远端，并确认配置文件中的 `rclone_remote` 可以正常上传。

## 配置文件

参考示例配置：

```bash
cp configs/config.example.yaml config.yaml
```

常用配置项：

- `poll_interval`：检测文件大小变化的间隔。
- `stable_duration`：文件稳定多久后开始处理。
- `retry_interval`：上传失败后的重试间隔。
- `max_retry_count`：单个文件最大连续失败次数，`0` 表示无限重试。
- `max_parallel_uploads`：并发上传任务数量。
- `extensions`：需要监控的文件后缀。
- `rclone_args`：传给 `rclone copy` 的参数。
- `proxy`：HTTP 代理配置，`enabled: true` 时生效。
- `telegram`：最终失败通知配置，`enabled: true` 时生效。
- `jobs`：任务列表。

每个 `job` 需要配置：

- `name`：任务名称。
- `source_dir`：监控源目录。
- `link_dir`：硬链接目录。
- `rclone_remote`：rclone 上传目标目录。
- `delete_source_after_upload`：上传成功后是否删除源文件。

程序默认读取可执行文件同目录的 `config.yaml`，也可以启动时指定：

```bash
./media-backup-linux-amd64 -config /path/to/config.yaml
```

## 构建

```bash
./build.sh
```

构建产物在 `dist/` 目录：

- `media-backup-linux-amd64`
- `media-backup-linux-arm64`
- `run-forever.sh`
- `install-systemd-service.sh`

## 运行脚本

进入 `dist/` 目录后运行：

```bash
./run-forever.sh -config /path/to/config.yaml
```

如果程序异常退出，脚本会等待 30 秒后自动重启。可通过环境变量修改重启等待时间：

```bash
MEDIA_BACKUP_RESTART_DELAY=10 ./run-forever.sh -config /path/to/config.yaml
```

## systemd 脚本

安装并启动服务：

```bash
sudo ./install-systemd-service.sh
```

卸载服务：

```bash
sudo ./install-systemd-service.sh -u
```

停止 30 秒后重新启动服务：

```bash
sudo ./install-systemd-service.sh -r
```
