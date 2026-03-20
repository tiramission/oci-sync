# oci-sync

`oci-sync` 是一个 Go 命令行工具：支持本地目录与 OCI 镜像之间的同步（本地存储 + 远程 Registry）。

## 目录结构

- `cmd/oci-sync/main.go`：CLI 入口
- `pkg/oci/sync.go`：核心逻辑（parse/ref、local push/pull、registry push/pull）
- `pkg/oci/sync_test.go`：单元测试
- `go.mod` / `go.sum`：模块依赖
- `README.md`：使用说明

## 编译

```bash
cd /Users/jaign/Codes/Github/tiramission/oci-sync
go build -o oci-sync ./cmd/oci-sync
```

## 用法

### 1) 协议判定规则

- `oci://...` 走 OCI 镜像存储（本地或远程 Registry）
- `file://<path-dir>:<image-name>` 走本地 OCI layout

### 2) 本地OCI模式

- 推送目录到本地 OCI 存储：
  ```bash
  ./oci-sync push --dir /from/path/dir oci://myrepo/myimage:1.0
  ```
- 从本地拉取：
  ```bash
  ./oci-sync pull oci://myrepo/myimage:1.0 --dir /to/path/dir
  ```

### 3) 本地 Layout (file://)

- 推送到本地指定目录 layout：
  ```bash
  ./oci-sync push --dir /from/path/dir file:///tmp/t_tools:myimage
  ```
- 从本地指定目录 layout 拉取：
  ```bash
  ./oci-sync pull file:///tmp/t_tools:myimage --dir /to/path/dir
  ```

本地存储位置：`<path-dir>/<image-name>/<tag>`，默认 tag 为 `latest`。

### 5) 远程 Registry 模式（支持域名/端口）

- 推送到远程 Registry：
  ```bash
  ./oci-sync push /from/path/dir oci://internal.183867412.xyz:5000/test/oci-sync:1.0
  ```
- 从远程 Registry 拉取：
  ```bash
  ./oci-sync pull oci://internal.183867412.xyz:5000/test/oci-sync:1.0 -d /to/path/dir
  ```

> 远程 Registry 会尝试 https/ http，如果网络不可达会提示连接错误。

## 设计说明

- `oci://` 引用由 `parseOCIRef` 解析
- 含域名或端口（`internal.183867412.xyz:5000`）时判定为远程 Registry
- 本地推拉为 tar 层与 manifest 存储，方便离线恢复
- 远程推拉基于 `go-containerregistry`（匿名、insecure 可用）

## 快速测试

```bash
mkdir -p /tmp/oci-sync-test/src
echo hello > /tmp/oci-sync-test/src/a.txt
./oci-sync push /tmp/oci-sync-test/src oci://myrepo/myimage:3.0
./oci-sync pull oci://myrepo/myimage:3.0 -d /tmp/oci-sync-test/dest
cat /tmp/oci-sync-test/dest/a.txt
```

## 单元测试

```bash
go test ./...
```

