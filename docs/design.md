# oci-sync 设计文档

> 版本：0.1.0 | 更新时间：2026-03-20

## 1. 项目概述

`oci-sync` 是一个 Go 语言命令行工具，将本地文件或目录以 **OCI artifact** 的形式同步到任意兼容 OCI Distribution Spec 的镜像仓库（Docker Hub、GHCR、Harbor、ACR 等），支持可选的 AES-256-GCM 加密。

### 使用场景

- 将配置文件、数据集、模型文件等任意内容存入 OCI 仓库进行版本管理
- 跨机器、跨环境分发文件，借助镜像仓库的权限管理做访问控制
- 敏感文件加密存储，密钥不离开本地

---

## 2. 整体架构

```
┌──────────────────────────────────────────────────────────┐
│                         CLI 层                            │
│ cmd/root.go   cmd/push.go   cmd/pull.go   cmd/delete.go  │
│ cmd/list.go                                              │
└──────────────┬───────────────┬──────────────┬────────────┘
               │              │
      ┌────────▼───┐    ┌─────▼────────┐
      │  archive   │    │     oci      │
      │  tar.gz    │    │  oras-go v2  │
      │  打包/解包  │    │  push/pull   │
      └────────────┘    └─────┬────────┘
                              │
      ┌─────────────┐   ┌─────▼────────────────┐
      │   crypto    │   │  Docker Credential    │
      │ AES-256-GCM │   │  Store (~/.docker/    │
      │ + scrypt    │   │  config.json)         │
      └─────────────┘   └──────────────────────┘
```

**数据流（push）**
```
本地路径 → [archive.Pack] → tar.gz bytes
         → [crypto.Encrypt]（可选）→ 加密 bytes
         → [oci.Push] → OCI manifest + layer → Registry
```

**数据流（pull）**
```
Registry → [oci.Pull] → layer bytes + annotations
         → [crypto.Decrypt]（若加密）→ tar.gz bytes
         → [archive.Unpack] → 本地路径
```

**数据流（delete）**
```
CLI 参数传入 → [oci.Delete] 解析 descriptor → Registry 删除 API
```

**数据流（list）**
```
CLI 参数传入 → [oci.List] (支持 Registry/Repo 自动解析)
             → [reg.Repositories] (若为 Registry) → 遍历 repos
             → [repo.Tags] → 遍历 tags → [repo.Fetch] 获取 Manifest
             → 过滤 io.oci-sync.version 标记 → 返回 ArtifactInfo 列表
             → 格式化表格输出 (显示 REPO, TAG, DIGEST 等)
```

---

## 3. 项目结构

```
oci-sync/
├── main.go                        # 程序入口
├── go.mod / go.sum
├── FEATURE.md                     # 产品需求文档
├── README.md                      # 使用文档
├── docs/
│   └── design.md                  # 本设计文档
├── cmd/
│   ├── root.go                    # 根命令 & 全局配置（--quiet / -q）
│   ├── push.go                    # push 子命令
│   ├── pull.go                    # pull 子命令
│   ├── delete.go                  # delete 子命令
│   ├── list.go                    # list 子命令
│   └── utils.go                   # 工具函数（formatBytes）
└── internal/
    ├── archive/
    │   ├── archive.go             # tar.gz 打包/解包
    │   └── archive_test.go        # 单元测试
    ├── crypto/
    │   ├── crypto.go              # AES-256-GCM 加密/解密
    │   └── crypto_test.go         # 单元测试
    └── oci/
        └── oci.go                 # OCI push/pull（oras-go v2）
```

---

## 4. 模块设计

### 4.1 `internal/archive` — 打包/解包

| 函数 | 签名 | 说明 |
|------|------|------|
| `Pack` | `(srcPath string) ([]byte, error)` | 将文件或目录打包为 tar.gz，返回字节 |
| `Unpack` | `(data []byte, destPath string) error` | 将 tar.gz 字节解包到指定目录 |

**实现细节**
- 使用标准库 `archive/tar` + `compress/gzip`，无额外依赖
- 目录打包保留完整的子目录结构，以 `filepath.Walk` 遍历
- 解包时进行路径穿越检查（Path Traversal Security）

### 4.2 `internal/crypto` — 加密/解密

| 函数 | 签名 | 说明 |
|------|------|------|
| `Encrypt` | `(data []byte, passphrase string) ([]byte, error)` | 加密数据 |
| `Decrypt` | `(data []byte, passphrase string) ([]byte, error)` | 解密数据 |

**加密算法**
- **KDF**：scrypt（N=32768, r=8, p=1）→ 32 字节密钥
- **加密**：AES-256-GCM（认证加密，同时提供保密性和完整性）
- **存储格式**：`[salt(32B) | nonce(12B) | ciphertext+GCM-tag]`
- 每次加密生成新的随机 salt 和 nonce，保证相同明文加密结果不同

```
passphrase ──┐
             ├─► scrypt(N=32768) ──► 32B key ──► AES-256-GCM ──► ciphertext
random salt ─┘                                        │
random nonce ────────────────────────────────────────►│
                                                      ▼
                              [salt(32B)][nonce(12B)][ciphertext+tag]
```

### 4.3 `internal/oci` — OCI 操作

| 函数 | 签名 | 说明 |
|------|------|------|
| `Push` | `(ctx, data []byte, ref string, encrypted bool) error` | 推送 artifact |
| `Pull` | `(ctx, ref string) (*PullResult, error)` | 拉取 artifact |
| `Delete` | `(ctx, ref string) error` | 删除远程 artifact |
| `List` | `(ctx, ref string) ([]ArtifactInfo, error)` | 列出远程仓库镜像记录（支持 Registry/Repo）|


**OCI Artifact 结构**
- 使用标准 OCI Image Manifest 格式，`schemaVersion: 2`
- Config mediaType：`application/vnd.oci.image.config.v1+json`（空 JSON `{}`）
- Layer mediaType：`application/octet-stream`（不定义自定义类型）
- Manifest Annotations 携带元信息：

| Annotation Key | 值 | 说明 |
|-----------------|-----|------|
| `io.oci-sync.encrypted` | `"true"` / `"false"` | 是否加密 |
| `io.oci-sync.version` | `"0.1.0"` | 工具版本 |

**数据结构 `ArtifactInfo`**
- `Repo`: 仓库名称（Registry 级列举时用于区分）
- `Tag`: 镜像标签
- `Digest`: 内容摘要
- `Encrypted`: 是否加密（布尔值）
- `Version`: 上传时的工具版本

**认证**
- 直接读取 `~/.docker/config.json`，通过 `oras-go v2` 内置的 `credentials.NewStoreFromDocker()` 加载凭据
- 自动支持 `credsStore` / `credHelpers`（macOS keychain、Windows Credential Manager、Linux secret service）
- 用户只需提前执行 `docker login <registry>` 即可

---

## 5. 依赖列表

| 包 | 版本 | 用途 |
|----|------|------|
| `github.com/spf13/cobra` | v1.10.2 | CLI 框架 |
| `oras.land/oras-go/v2` | v2.6.0 | OCI push/pull |
| `github.com/opencontainers/image-spec` | v1.1.1 | OCI 数据结构 |
| `github.com/opencontainers/go-digest` | v1.0.0 | 内容摘要计算 |
| `github.com/charmbracelet/log` | v1.0.0 | 彩色日志输出 |
| `golang.org/x/crypto` | v0.49.0 | scrypt KDF |

---

## 6. 安全考量

| 风险 | 缓解措施 |
|------|---------|
| 密码暴力破解 | scrypt 高内存消耗（N=32768）增加破解成本 |
| 重放/篡改攻击 | AES-GCM 提供认证标签，解密失败立即报错 |
| Nonce 重用 | 每次加密随机生成 nonce，与 salt 一起存储 |
| Path traversal | Unpack 时检查解包路径必须在目标目录内 |
| 凭据泄露 | 凭据通过系统 credential store 管理，不写入磁盘 |

---

## 7. CLI 接口

### push

```bash
oci-sync push --local <local_path> --remote <remote_path> [--passphrase <passphrase>]
# 或使用简写
oci-sync push -l <local_path> -r <remote_path> [--passphrase <passphrase>]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--local`, `-l` | ✓ | 本地文件或目录路径 |
| `--remote`, `-r` | ✓ | 目标仓库引用，格式：`<registry>/<repo>:<tag>` |
| `--passphrase` | 否 | 加密口令，不提供则不加密 |
| `--quiet`, `-q` | 否 | 静默模式，全局生效 |

### pull

```bash
oci-sync pull --remote <remote_path> --local <local_path> [--passphrase <passphrase>]
# 或使用简写
oci-sync pull -r <remote_path> -l <local_path> [--passphrase <passphrase>]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--remote`, `-r` | ✓ | 源仓库引用，格式：`<registry>/<repo>:<tag>` |
| `--local`, `-l` | ✓ | 本地目标目录 |
| `--passphrase` | 否 | 解密口令（内容加密时必须提供） |

### delete

```bash
oci-sync delete --remote <remote_path>
# 或使用简写
oci-sync delete -r <remote_path>
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--remote`, `-r` | ✓ | 目标仓库引用，格式：`<registry>/<repo>:<tag>` |

### list

```bash
# 列出特定仓库的 tags
oci-sync list --remote <registry>/<repository> [-q]
# 列出整个注册表中的所有 oci-sync 镜像
oci-sync list -r <registry> [-q]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--remote`, `-r` | ✓ | 目标仓库源或注册表，格式：`<registry>/<repo>` 或 `<registry>` |
| `--quiet`, `-q` | 否 | 静默模式，全局生效 |

---

## 8. 后续扩展方向

- **`--insecure`**：支持 HTTP（非 TLS）仓库
- **`--platform`**：多架构 manifest list 支持
- **进度条**：大文件上传/下载显示进度
- **增量同步**：对比 digest 跳过未变更内容
- **多文件 layer**：多个 layer 对应多个文件，支持细粒度更新
