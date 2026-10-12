# oci-sync 设计文档

> 版本：0.8.0 | 更新时间：2026-10-12

## 1. 项目概述

`oci-sync` 是一个 Go 语言命令行工具，将本地文件或目录以 **OCI artifact** 的形式同步到任意兼容 OCI Distribution Spec 的镜像仓库（Docker Hub、GHCR、Harbor、ACR 等），支持可选的 AES-256-GCM 加密，并记录所有操作到本地 activity cache。

### 使用场景

- 将配置文件、数据集、模型文件等任意内容存入 OCI 仓库进行版本管理
- 跨机器、跨环境分发文件，借助镜像仓库的权限管理做访问控制
- 敏感文件加密存储，密钥不离开本地

---

## 2. 整体架构

```
┌──────────────────────────────────────────────────────────┐
│                         CLI 层                            │
│ cmd/root.go   cmd/push.go   cmd/pull.go   cmd/shortcut.go│
│ cmd/delete.go cmd/list.go   cmd/label.go  cmd/alias.go   │
│ cmd/recent.go cmd/tui.go   cmd/gui.go   cmd/gui_app.go   │
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
                               │
                      ┌────────▼────────────────┐
                      │       cache            │
                      │  ~/.cache/oci-sync/    │
                      │   activity.json       │
                      └───────────────────────┘
```

**数据流（push）**
```
本地路径 → [archive.Pack] → tar.gz 流
         → [crypto.EncryptWriter]（可选）→ 分块密文流
         → 落地为 staging 文件（算 digest）
         → [oci.PushFile] → OCI manifest + layer → Registry
```

**数据流（pull）**
```
Registry → [oci.IsEncrypted] → 检查加密状态（manifest 只读）
         → 校验 --passphrase 参数（若缺失则快速失败，无需下载）
         → [oci.PullToFile] → layer 流入 staging 文件
         → [crypto.DecryptReader]（若加密）→ tar.gz 流
         → [archive.Unpack] → 本地路径
```

push 与 pull 全程流式：内存峰值是一个 1 MiB 分块，与 artifact 大小无关。

**数据流（shortcut commands）**
```
CLI 参数传入（--tag + shortcuts.<name>.repo 配置）
         → 拼装完整 remote ref
         → 复用标准 [push]/[pull]/[delete] 数据流

CLI 参数传入（shortcuts.<name>.repo 配置）
         → 解析 repository
         → 复用标准 [list] 数据流
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
             → 支持 --label 筛选 → 格式化表格输出 (显示 REPO, TAG, DIGEST, LABELS 等)
```

**数据流（label set/unset）**
```
CLI 参数传入 → [oci.UpdateAnnotations] 获取 manifest
           → 修改 annotations (set 添加/更新, unset 删除指定 key)
           → Push 新 manifest → 更新 tag 指向新 digest
```

**数据流（alias add/remove）**
```
CLI 参数传入 → 读写配置文件 ~/.config/oci-sync/oci-sync.yaml
           → 修改 shortcuts 字段 → 保存
```

**数据流（recent）**
```
CLI 参数传入 → 读取 ~/.cache/oci-sync/activity.json
           → 支持 --limit 限制数量、--format 指定格式 (table/json/yaml)
           → 支持 --clear 清空历史记录
```

**数据流（tui）**
```
CLI 启动 → 全屏分栏交互界面（焦点模型：Shortcuts / Artifacts 二选一；<76 列折叠为单栏）
        → Enter 加载所选 shortcut 仓库 → [oci.List]（Cmd 协程异步执行，Esc 可取消，过期结果按 seq 丢弃）
        → P 推送（表单）/ p 拉取 / d 删除 / e 编辑标签 / s 切换排序
        → 拉取实时显示下载进度条（字节/百分比/子块精度）与阶段（check/download/decrypt/unpack），结束后以结果弹窗展示并可重试
```

**数据流（activity recording）**
```
CLI push/pull/delete/label 操作成功
           → 写入 ~/.cache/oci-sync/activity.json
           → 记录类型、时间戳、远程引用、本地路径、标签、操作结果
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
│   ├── design.md                  # 本设计文档
│   └── ui/
│       └── ui-design-system.html  # TUI/GUI 设计规范与视觉稿（自包含单文件）
├── cmd/
│   ├── root.go                    # 根命令 & 全局配置（--quiet / -q）
│   ├── push.go                    # push 子命令
│   ├── pull.go                    # pull 子命令
│   ├── stage.go                   # push/pull 共用的流式 staging 链
│   ├── shortcut.go                # 动态 shortcut 子命令组
│   ├── delete.go                  # delete 子命令
│   ├── list.go                    # list 子命令
│   ├── label.go                   # label 子命令 (set/unset)
│   ├── alias.go                   # alias 子命令 (list/add/remove)
│   ├── recent.go                  # recent 子命令（查看活动历史）
│   ├── tui.go                     # tui 子命令：model 状态机与命令派发
│   ├── tui_view.go                # tui 渲染：View/overlay/布局计算
│   ├── tui_keys.go                # tui 按键绑定
│   ├── gui.go                     # gui 子命令：cobra 定义与窗口创建
│   ├── gui_app.go                 # gui 应用：状态、后台任务、视图函数
│   ├── gui_contrast.go            # gui 文本可读性：对比度校正辅助函数
│   ├── utils.go                   # 共用工具（formatBytes / label 解析 / 活动记录）
│   ├── utils_test.go              # label 校验与解析单元测试
│   ├── shortcut_test.go           # shortcut 引用构造单元测试
│   ├── tui_test.go                # tui 状态机与渲染单元测试
│   ├── gui_test.go                # gui 状态逻辑与 headless 渲染测试
│   └── gui_contrast_test.go       # 对比度阈值与「不过度校正」测试
└── internal/
    ├── config/
    │   ├── config.go              # 配置文件支持
    │   └── config_test.go         # 单元测试
    ├── archive/
    │   ├── archive.go             # tar.gz 打包/解包
    │   └── archive_test.go        # 单元测试
    ├── crypto/
    │   ├── crypto.go              # AES-256-GCM 分块流式加密/解密
    │   └── crypto_test.go         # 单元测试
    ├── oci/
    │   ├── oci.go                 # OCI push/pull/list（oras-go v2）
    │   └── oci_test.go            # fanOut 并发单元测试
    ├── cache/
    │   ├── cache.go               # Activity cache 持久化
    │   └── cache_test.go          # 单元测试
    ├── xdg/
    │   ├── xdg.go                 # XDG 目录规范支持
    │   └── xdg_test.go            # 单元测试
    └── version/
    │   └── version.go             # 版本信息

---

## 4. 模块设计

### 4.1 `internal/archive` — 打包/解包

| 函数 | 签名 | 说明 |
|------|------|------|
| `Pack` | `(w io.Writer, srcPath string) error` | 将文件或目录打包为 tar.gz 写入 w（文件与目录都支持） |
| `PackBytes` | `(srcPath string) ([]byte, error)` | `Pack` 的内存包装 |
| `Unpack` | `(r io.Reader, destPath string) error` | 从 r 读取 tar.gz 并解包到指定目录 |
| `UnpackBytes` | `(data []byte, destPath string) error` | `Unpack` 的内存包装 |

**实现细节**
- 使用标准库 `archive/tar` + `compress/gzip`，无额外依赖
- 目录打包保留完整的子目录结构，以 `filepath.Walk` 遍历
- tar 成员名统一用 `/` 分隔（`filepath.ToSlash`）：`filepath.Join` 在 Windows 上产生 `\`，会导致跨平台 push → pull 时目录结构塌陷为扁平文件名
- 符号链接按目标内容打包为普通文件（Unpack 只落地普通文件与目录，若存链接目标则 round-trip 后全变空文件）；指向目录的链接跳过，避免子树重复
- 顶层目录名由 `memberBase` 决定，不能直接用 `filepath.Base`：它对 `.` 返回 `.`、对 `..` 返回 `..`，
  于是打包出的成员名是 `./file` 或 `../file`——push 照样成功（写 tar 不拒绝任何名字），
  但每次 pull 都会因 `illegal file path in archive` 失败。**能推不能拉的 push 不算 push**，
  这条只在用户最可能手输的路径上暴露。点路径因此改用 `filepath.Abs` 解析出的真实目录名；
  文件系统根（`/`、`C:\`）没有名字可退，直接拒绝
- 解包时进行路径穿越检查（Path Traversal Security）；以 `/` 开头的成员名会被 `filepath.Join` 折入目标目录内部，属于安全收敛而非拒绝
- 打包与解包都是流式的；`*Bytes` 变体只保留给测试和不需要流式的调用方

### 4.2 `internal/crypto` — 分块流式加密/解密

| 函数 | 签名 | 说明 |
|------|------|------|
| `NewEncryptPassphrase` | `(w io.Writer, passphrase string) (*EncryptWriter, error)` | 生成随机 salt、派生密钥，返回写入 w 的加密 writer |
| `NewEncryptWriter` | `(w io.Writer, key, salt []byte) (*EncryptWriter, error)` | 由已派生的密钥与 salt 构造 |
| `NewDecryptPassphrase` | `(r io.Reader, passphrase string) (*DecryptReader, error)` | 读取流头中的 salt，派生密钥并校验 magic |
| `ErrNotChunked` | — | 流缺少 `"OCS1"` magic，用于区分「不是我们的格式」与「我们的格式损坏了」 |

`EncryptWriter` 必须 `Close`：它负责写出流头、封存最后一个分块、追加终止符。`Close` 不是幂等的，二次调用报错而不是写出第二个终止符——那样的流读不回来。

**流格式**
```
"OCS1" | salt(32B) | ( u32be ciphertextLen | ciphertext+tag )*
```
长度为 0 的分块即终止符，读端因此不必猜测最后一个分块在哪里。

**加密算法**
- **KDF**：scrypt（N=32768, r=8, p=1）→ 32 字节密钥
- **加密**：AES-256-GCM（认证加密，同时提供保密性和完整性）
- **分块大小**：`ChunkSize = 1 MiB`（`crypto.ChunkSize`）。这是内存上界：加解密两侧任何时刻各只持有一个分块。1 MiB 下每块 20 字节开销（4 字节长度 + 16 字节 tag）远低于 0.01%
- **nonce**：每个分块的 nonce 仅由块索引派生。同一密钥下重用 nonce 会破坏 GCM，但密钥本身来自随机 32 字节 salt，因此跨流永不复用；流内索引唯一，这正是 GCM 实际要求的条件
- **AAD**：`"OCS1" | u32be(index)`，把分块位置绑定到明文上。分块被移到另一个位置或另一条流都会认证失败
- 每次加密生成新的随机 salt，相同明文两次加密结果不同

**格式不兼容**：旧的一次性 `Encrypt([]byte)` / `Decrypt([]byte)` 已删除，旧版本写出的密文无法用当前代码读回。这是有意的取舍——保留旧格式意味着在一个已经分块化的包里再养一条一次性路径及其一整套测试。

**staging 文件**：`cmd/stage.go` 把 pack →（加密）→ 落盘串成一条链。中间落地不是优化，而是硬性要求：Registry 按内容 digest 寻址 blob，digest 必须在上传开始前已知，而刚打好的 tar.gz、刚加密完的流都还不知道自己的 digest。文件就是那个「某处」，它花的是磁盘而不是内存。

```
passphrase ──┐
             ├─► scrypt(N=32768, salt) ──► 32B key ──► AES-256-GCM ──► 分块密文流
random salt ─┘                                    key 每次加密都不同，
                                                  因此 nonce(index) 无需跨流唯一
```

### 4.3 `internal/oci` — OCI 操作

| 函数 | 签名 | 说明 |
|------|------|------|
| `Push` | `(ctx, data []byte, ref string, encrypted bool, labels map[string]string) error` | 推送 artifact（内存版，支持 labels） |
| `PushFile` | `(ctx, path, ref string, encrypted bool, labels map[string]string) error` | 从文件推送：先算 digest 与大小，再流式上传 |
| `IsEncrypted` | `(ctx, ref string) (bool, error)` | 检查加密状态（仅拉取 manifest） |
| `Pull` | `(ctx, ref string, onRead func(done, total int64)) (*PullResult, error)` | 拉取 artifact 到内存；`onRead` 按下载块回报字节进度（可为 nil） |
| `PullToFile` | `(ctx, path, ref string, onRead func(done, total int64)) (bool, error)` | 流式下载 layer 到文件，返回是否加密 |
| `Delete` | `(ctx, ref string) error` | 删除远程 artifact |
| `List` | `(ctx, ref string) ([]ArtifactInfo, error)` | 列出远程仓库镜像记录（支持 Registry/Repo）|
| `UpdateAnnotations` | `(ctx, ref string, updates map[string]string, removeKeys []string) error` | 更新 manifest annotations（set/unset labels）|

`List` 在 registry 级会为每个仓库、每个 tag 发起 `Resolve` + `Fetch` 两次往返，串行执行
时是 N+1 瓶颈。`fanOut` 以 8 路为上限并发执行并**按输入下标归位**，因此输出顺序稳定可复现；
部分条目失败时仍返回成功的结果，只有全军覆没才返回聚合错误（`errors.Join`），
这样单个损坏的仓库不会让整次列举失败。


**OCI Artifact 结构**
- 使用标准 OCI Image Manifest 格式，`schemaVersion: 2`
- Config mediaType：`application/vnd.oci.image.config.v1+json`（空 JSON `{}`）
- Layer mediaType：`application/octet-stream`（不定义自定义类型）
- Manifest Annotations 携带元信息：
  - 系统保留 annotation：`io.oci-sync.encrypted`、`io.oci-sync.version`
  - 用户自定义 labels：存储于 annotations，可通过 `--label` 设置

| Annotation Key | 值 | 说明 |
|-----------------|-----|------|
| `io.oci-sync.encrypted` | `"true"` / `"false"` | 是否加密 |
| `io.oci-sync.version` | `"0.8.0"` | 工具版本（`internal/version`）|
| 用户自定义 | 任意字符串 | 通过 `--label key=value` 设置 |

`io.oci-sync.*` 前缀为系统保留命名空间：`label set` / `label unset` 会拒绝此类 key，
`Push` / `UpdateAnnotations` 也会在写入后强制回填系统注解。因此加密标记与版本号
无法通过标签被篡改或删除。

**数据结构 `ArtifactInfo`**
- `FullName`: 镜像全名，格式为 `<registry>/<repo>:<tag>`（用于 JSON/YAML 输出）
- `Repo`: 仓库名称
- `Tag`: 镜像标签
- `Digest`: 内容摘要
- `Encrypted`: 是否加密（布尔值）
- `Version`: 上传时的工具版本
- `Labels`: 用户自定义标签映射（`map[string]string`）

**认证**
- 支持两种认证方式（按顺序查找，找到即用）：
  - **配置文件** (`auths.<registry>.username/password`)：支持为每个仓库配置独立凭据
  - **Docker credential store**：通过 `oras-go v2` 内置的 `credentials.NewStoreFromDocker()` 加载
- 自动支持 `credsStore` / `credHelpers`（macOS keychain、Windows Credential Manager、Linux secret service）
- 用户只需提前执行 `docker login <registry>` 即可

### 4.4 `internal/cache` — Activity Cache

| 函数 | 签名 | 说明 |
|------|------|------|
| `InitCache` | `() error` | 初始化 cache 目录（`~/.cache/oci-sync/`） |
| `AddActivity` | `(Activity) error` | 添加活动记录 |
| `GetRecentActivities` | `(limit int) ([]Activity, error)` | 获取最近活动 |
| `ClearActivities` | `() error` | 清空所有活动记录 |

**Activity 数据结构**
```go
type Activity struct {
    Type      ActivityType  // push/pull/delete/label
    Timestamp time.Time     // 操作时间
    RemoteRef string        // 远程引用
    LocalPath string        // 本地路径（push/pull）
    Labels    []string      // 标签（label 操作）
    Success   bool          // 是否成功
    Error     string        // 错误信息
}
```

**存储位置**：遵循 XDG Base Directory Spec
- 默认：`~/.cache/oci-sync/activity.json`
- 支持 `XDG_CACHE_HOME` 环境变量覆盖

`Save` 先写同目录临时文件再 `os.Rename` 原子替换：直接 `WriteFile` 在进程中途退出时会留下截断的 JSON，导致之后每次 `Load` 都失败。`GetStats` 返回的 `Recent` 是拷贝而非切片别名，避免排序时改写到已加载数据。

### 4.5 `internal/xdg` — XDG 目录规范

| 函数 | 签名 | 说明 |
|------|------|------|
| `ConfigDir` | `() string` | 配置目录（`$XDG_CONFIG_HOME` 或 `~/.config`） |
| `CacheDir` | `() string` | 缓存目录（`$XDG_CACHE_HOME` 或 `~/.cache`） |
| `DataDir` | `() string` | 数据目录（`$XDG_DATA_HOME` 或 `~/.local/share`） |

home 目录解析顺序：`$HOME` → `os.UserHomeDir()`（Windows 的 `USERPROFILE`）→ `os.TempDir()`。
Windows 默认不设置 `HOME`，缺少回退会让上述目录退化为相对路径，配置与缓存将被写到当前工作目录。

配置查找顺序为当前目录 → XDG 配置目录。`alias add` / `alias remove` 写回时遵循同一顺序：
若已有生效的配置文件则原地改写，否则创建到 XDG 位置——读写路径不一致会导致别名写入后无法被读到。

---

## 5. 依赖列表

| 包 | 版本 | 用途 |
|----|------|------|
| `github.com/spf13/cobra` | v1.10.2 | CLI 框架 |
| `oras.land/oras-go/v2` | v2.6.0 | OCI push/pull/list/delete |
| `github.com/opencontainers/image-spec` | v1.1.1 | OCI 数据结构 |
| `github.com/opencontainers/go-digest` | v1.0.0 | 内容摘要计算 |
| `github.com/pterm/pterm` | v0.12.83 | 表格格式化输出 |
| `charm.land/log/v2` | v2.0.0 | 彩色日志输出 |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | TUI 框架 |
| `github.com/charmbracelet/bubbles` | v1.0.0 | TUI 组件（input/spinner/help/key）|
| `github.com/charmbracelet/lipgloss` | v1.1.0 | TUI 样式与布局 |
| `github.com/mattn/go-runewidth` | v0.0.23 | 终端显示宽度（CJK 对齐）|
| `golang.org/x/crypto` | v0.49.0 | scrypt KDF |
| `gopkg.in/yaml.v3` | v3.0.1 | 配置文件解析 |
| `github.com/stretchr/testify` | v1.11.1 | 测试断言 |
| `github.com/egoist/mygo` | v0.4.2 | 桌面 GUI 框架（纯 Go，无 cgo）|

Go 工具链：`go 1.27.1`（mygo v0.4.2 的硬性要求，不可降低）。

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
oci-sync push --local <local_path> --remote <remote_path> [--passphrase <passphrase>] [--label <key=value>]
# 或使用简写
oci-sync push -l <local_path> -r <remote_path> [--passphrase <passphrase>] [--label <key=value>]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--local`, `-l` | ✓ | 本地文件或目录路径 |
| `--remote`, `-r` | ✓ | 目标仓库引用，格式：`<registry>/<repo>:<tag>` |
| `--passphrase` | 否 | 加密口令，不提供则不加密 |
| `--label` | 否 | 设置标签，可重复使用（格式：`key=value`，value 可为空） |
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

### <name> push

```bash
oci-sync <name> push --local <local_path> --tag <tag> [--passphrase <passphrase>] [--label <key=value>]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--local`, `-l` | ✓ | 本地文件或目录路径 |
| `--tag` | ✓ | 目标标签 |
| `--passphrase` | 否 | 加密口令，不提供则不加密 |
| `--label` | 否 | 设置标签，可重复使用（格式：`key=value`，value 可为空） |

### <name> pull

```bash
oci-sync <name> pull --tag <tag> --local <local_path> [--passphrase <passphrase>]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--tag` | ✓ | 源标签 |
| `--local`, `-l` | ✓ | 本地目标目录 |
| `--passphrase` | 否 | 解密口令（内容加密时必须提供） |

### <name> list

```bash
oci-sync <name> list [--format table|json|yaml] [--label <key>=<value>]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--format`, `-f` | 否 | 输出格式：`table`（默认）、`json`、`yaml` |
| `--label` | 否 | 筛选标签（`key=value` 精确匹配，`key` 仅检查 key 存在），可重复 |

### <name> delete

```bash
oci-sync <name> delete --tag <tag>
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--tag` | ✓ | 要删除的目标标签 |

### 配置文件

配置文件使用 YAML 格式，搜索路径顺序：
1. 当前工作目录 `./oci-sync.yaml`
2. 用户配置目录 `~/.config/oci-sync/oci-sync.yaml`

**配置文件格式：**
```yaml
shortcuts:
  <name>:
    repo: <registry>/<repository>
  x:
    repo: <registry>/<repository>

auths:
  <registry1.example.com>:
    username: <username>
    password: <password>
  <registry2.example.com>:
    username: <username>
    password: <password>
```

**配置说明：**

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `shortcuts.<name>.repo` | - | 动态命令的默认仓库地址 |
| `auths.<registry>.username` | - | 该仓库的认证用户名 |
| `auths.<registry>.password` | - | 该仓库的认证密码或令牌 |

**认证优先级：** 配置文件 `auths` > Docker credential store

### delete

```bash
oci-sync delete --remote <remote_path>
# 或使用简写
oci-sync delete -r <remote_path>
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--remote`, `-r` | ✓ | 目标仓库引用，格式：`<registry>/<repo>:<tag>` |

### label

管理 OCI artifact 上的标签（存储于 manifest annotations）。

```bash
# 设置标签
oci-sync label set --remote <remote_path> <key1=value1> [<key2=value2>...]

# 删除标签
oci-sync label unset --remote <remote_path> <key1> [<key2>...]
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--remote`, `-r` | ✓ | 目标仓库引用，格式：`<registry>/<repo>:<tag>` |
| `key=value` | ✓（set） | 设置标签，value 可为空字符串 |
| `key` | ✓（unset） | 删除指定标签 |

### alias

管理配置文件中的 shortcuts。

```bash
# 列出所有 shortcuts
oci-sync alias list

# 添加 shortcut
oci-sync alias add <name> --repo <registry>/<repository>

# 删除 shortcut
oci-sync alias remove <name>
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--repo` | ✓（add） | shortcut 对应的仓库地址 |

**注意**：若配置文件不可写，会输出警告但不会报错。

### recent

查看本地 activity cache 中记录的操作历史。

```bash
# 查看最近 20 条活动（默认）
oci-sync recent

# 指定显示数量
oci-sync recent --limit 10

# 指定输出格式
oci-sync recent --format json
oci-sync recent --format yaml

# 清空所有活动记录
oci-sync recent --clear
```

| 参数 | 必选 | 说明 |
|------|------|------|
| `--limit`, `-n` | 否 | 最大显示条数，默认 20 |
| `--format`, `-f` | 否 | 输出格式：`table`（默认）、`json`、`yaml` |
| `--clear` | 否 | 清空所有活动记录 |

**存储位置**：`~/.cache/oci-sync/activity.json`（支持 `XDG_CACHE_HOME` 环境变量）

### tui

启动全屏 TUI 交互界面，分栏管理 shortcuts 的 artifacts。

```bash
# 启动 TUI 界面
oci-sync tui
```

**界面分区**：
- **Header（顶部一行）**：应用名以反白 chip 呈现（`  oci-sync  `，设计稿的高亮块）、当前/待定仓库路径（超宽中间省略），右对齐显示 tag 计数或 `n/m`（过滤时）以及加载 spinner。`/` 过滤进行中，右上角显示正在输入的查询（`/v▌`）——输入框不可见时每个按键都是猜测。
- **Shortcuts（左侧边栏）**：配置的 shortcuts，`▸` 标记当前项，反白表示焦点所在行，未选中行右侧附 repo 路径；底部页脚显示已加载仓库与计数（`ghcr.io/acme/configs · 14`）。聚焦栏标题用强调色。终端宽度 `<76` 时与其它面板折叠为单栏。
- **Artifacts（右侧主栏）**：标题为 `Artifacts · <repo>`；tag 表格（`TAG`、`SIZE`、`ENC`、`VER`），数字右对齐，超宽截断并加省略号，宽度不足时自动隐藏 `VER` 列。`ENC` 列是扫视中唯一值得被一眼捕捉的信息：加密显示加粗警示色的 `enc`，未加密为次要色的 `·`（文字+颜色双通道）。底部页脚 `7 of 14 shown` 说明窗口能放下多少行。聚焦栏标题用强调色。
- **加载态（骨架行）**：`oci.List` 进行中，表格在真实列头下渲染 6 行 `░` 骨架行而非一行 spinner——刷新时高度不塌陷再撑开；spinner 与 `esc cancels` 提示在 Header。
- **焦点用边框粗细表达，不用颜色**：当前栏画双线边框（`┃`），非当前栏画单线圆角边框（`│`）。终端没有半像素边框，1.5px 只能近似成「双线 vs 单线」；而粗细是余光能捕捉的信号，两个相近的紫色之间的换色不是。颜色仍随焦点变化，但只作强化。弹窗与表单接管键盘时**两侧都不画焦点**——背后的界面不能宣称一个用户此刻并不持有的焦点。
- **Footer（底部提示栏）**：按当前焦点/模态上下文自动生成快捷键提示（来自 `bubbles/help`），格式即设计稿的一行 `↑/k ↓/j: navigate · enter: open · tab: pane · P: push · p: pull · d: delete · ?: help · q: quit`：`key: desc` 条目、`·` 分隔、整体次要色。`e` / `/` / `r` / `esc` 等次级键位收在 `?` 键位表里——80 列下两者只能取其一，而静默丢掉一半的 footer 教不会任何人少的是哪个键。
- **详情视图（enter）**：宽表格的兜底手段——在 artifact 上按 enter 弹出居中详情，展示 Full Name、Digest、Version、Size、Encryption、Labels（键排序）。
- **模态弹窗（居中单框）**：路径输入、密码提示、删除确认（默认 No，正文显示完整 `repo:tag` 而非 shortcut 缩写——那行字上面就是不可撤销的警告）、执行结果、`?` 帮助，均以单层圆角框呈现。
- **三态区分**：产物表为空有三种成因，文案必须分开——未加载仓库（提示选仓库）、仓库为空（提示按 `P` 推送）、被过滤筛空（提示清过滤）。把后两者混为一谈会给出方向相反的下一步。
- **footer 宽度**：footer 是唯一不受 `truncate` 约束的部分，但宽度只在这里判一次。`bubbles/help` 的 `ShortHelpView` 会自行截断到 `help.Width` 并以省略号收尾，而它内部的宽度是 locale 相关的——所以走 `untruncatingHelp`（`Width = 0`）取不截断的渲染，由 `fitBindings` 用 `cellWidth` 从尾部丢绑定，**丢过东西才补 `? more` 指针**（宽度够放下全部键位时指针指向的键位表就在屏幕上，是多余的）。`clampHelp` 只作兜底。少了那个标记，footer 会在用户以为「就这些键」时静默少掉一半。

- **终端宽度必须钉死，不能继承 locale**：东亚歧义宽度字符（`↑` `↓` `•` `→` `…`）在西文终端算 1 列、CJK 终端算 2 列，`lipgloss.Width` 与 `runewidth.DefaultCondition` 都跟着 locale 走。同一句 footer 在 help 里量 90 列、在别处量 100 列，结果是撑破窗口（终端滚动）或被截成 `? …`（键位提示全丢），且两种机器表现不同——本地全绿、用户失败就是这么来的。因此：`cmd/tui.go` 的 `cellCond` 固定按 1 列；`cellWidth` 自剥 ANSI 可直接量已染色字符串；`place`/`joinVertical` 自己实现而非用 `lipgloss.Place`/`JoinVertical`，因为组合器也必须用同一把尺；`help.Width` 对 footer 而言一律置 0。改布局后必须 `RUNEWIDTH_EASTASIAN=0` 与 `=1` 各跑一遍测试。

**面板内容还有第二把尺：`wrapCond`**。面板的框是 lipgloss 画的，而 lipgloss 会对任何它量得比框宽的行做**自动折行**——它的量法同样跟着 locale（CJK 下 `…` `·` `↑` `↓` 算 2 列）。于是「按 `cellWidth` 正好铺满一行」的面板内容，在 CJK 终端上被量成超宽、折到第二行，整帧比窗口高出一两行——**只有 CJK 机器会滚动**。因此面板内的截断/补位用 `truncatePane` / `truncateMiddlePane` / `padWrap`，按 `wrapCond`（钉死的 CJK 最坏情况，两 locale 通吃）计费；表格最后一列吸收整行的歧义字符（`·`、`…`）留下的欠账，前面各列保持精确列宽，列对齐不受影响；表头排序标记选用单格宽的 `▴`/`▾` 而非歧义的 `↑`/`↓`（同 `▸`），表头因此永远精确。`truncate`/`padRight` 仍是 `cellWidth` 尺，服务 frame 之外的 header、footer、弹窗与组合器——**两把尺各管各的领地，不要混用**。

**响应式下限**：`terminal too small — need at least 48×12`，低于此不渲染花屏。

**渲染高度必须等于窗口高度**：宽度有测试而高度没有，这个不对称正是溢出溜过去的原因。面板的
`Height()` 只算内容，所以预算要减掉 header 1 行、footer 1 行和上下边框 2 行（`paneHeight`
= `m.height-4`）。漏掉哪一行，终端就会滚动——header 滚出顶部、footer 被切一半，而每一行的
**宽度**仍然合规，宽度测试全绿。

**弹窗高度上限**：`lipgloss.Place` 只会居中它拿到的东西，比窗口高的弹窗不是「居中得不好」，
而是直接跑出底部。实测 `MaxHeight` 限制的是**整个框（含边框）**，而 `Padding(1)` 在它之前
生效，所以正文预算是 `height - 2边框 - 2padding`。正文装不下时从**尾部**裁剪字段，
`enter/esc: close` 那行永远保留——少一个字段是小事，少了关闭方式用户就出不来了。

**字段标签要留间隙**：`padRight(s, 10)` 遇到正好 10 字符的标签（`repository`）不补空格，
标签和值就连在一起，而这恰好是列表里的第一个字段。

**快捷键**：
- `Tab` / `shift+Tab` / `1` / `2`：在 Shortcuts 与 Artifacts 栏之间切换焦点
- `↑/k` `↓/j` / `g` / `G`：栏内导航、跳顶、跳底（`j`=下、`k`=上，符合 vim 约定）
- `Enter`（Shortcuts 栏）：加载选中仓库；（Artifacts 栏）：打开详情
- `P`（大写）：推送，作用域是**仓库**——未选中 artifact 时仍可用，这是它与 `p` / `d` 的根本差别
- `p`：拉取所选 artifact（弹窗输入路径，加密再输密码）
- `d`：删除所选 artifact（默认 No 的确认弹窗）
- `e`：编辑所选 artifact 的标签，`key=value` 设置、裸 key 移除
- `s`：切换排序——registry 顺序 → 大小（降序）→ 标签（不区分大小写升序）→ 回到 registry 顺序；排序列在表头以 `▾`/`▴` 标记，选中行跟随其 tag 移动。设计稿的键位表写作「时间/大小/标签」，但 registry 不返回任何时间戳（tag 无 created 字段、空 config blob 也没有创建时间），时间排序无从谈起，周期覆盖数据实际有的两种
- `r`：重新加载当前 tag 列表
- `/`：按 tag 过滤（smart-case：小写不区分大小写，含大写则区分）
- `Esc`：加载/推送/拉取/删除/改标签进行中取消操作，否则关闭弹窗或退回 Shortcuts 栏
- `?`：打开/关闭帮助；`q` / `Ctrl+C`：退出

**push 用大写 `P` 而非小写**：push 与 pull 作用域相同（都作用于当前仓库），footer 里也相邻，
共用一个键位会让肌肉记忆的滑动直接作用在不可撤销的写操作上。测试
`TestPushAndPullUseDistinctKeys` 锁住这一点。

**推送表单**：四行固定字段（source / tag / passphrase / labels），`tab` 与 `shift+tab` 切换，
`enter` 提交。三个设计约束：

- **只有聚焦行挂在活的 `textinput` 上**，其余行渲染 `pushForm.fields` 里的字符串。多个
  textinput 同时存在会争抢光标状态；单输入则保证 tab 切换时输入内容不会丢。
- **口令失焦即掩码**：`bubbles/textinput` 的 `EchoPassword` 仍然按字符数画点，泄露长度；
  `maskSecret` 对非聚焦行统一画固定 8 个点。聚焦行同样不画 input 视图，否则用户看不到
  输入位置。
- **一次性校验全部字段**：`pushForm.validate` 一次报出所有问题，错误贴在表单底部，
  而不是让用户提交一次看一个错误。

标签编辑复用 `runLabelSet` / `runLabelUnset`：`=` 的进 set，裸 key 进 unset，set 先于 unset。

### gui

启动原生桌面窗口（MyGo），提供与 TUI 等价的浏览 / 拉取 / 推送 / 删除能力。

```bash
# 启动桌面 GUI
oci-sync gui
```

**实现方式**：`mygo.App.Run()` 在 cobra 的 `RunE` 内调用——它虽阻塞，但 `App.Quit()` /
`App.Exit(code)` 可从外部结束，因此与 CLI 共处一个 binary 而无需拆分。窗口关闭后命令返回，
退出码即 `gui` 的退出码。

**界面语言**：GUI 内所有用户可见文案为**中文**，CLI 与日志保持英文（见 AGENTS.md）。
因此文案就地写在 `cmd/gui_app.go`，不抽共享表——跨两者的错误得选一种语言或两种都带。

**界面分区**：
- **Toolbar（顶部）**：应用名、tag 过滤输入框、加载指示、`刷新` 按钮。
- **快捷方式（左侧边栏）**：配置的 shortcuts，选中行以主题 `Selection` 色高亮。
- **产物（右侧主栏）**：当前 shortcut 的 tag 表格（`标签`、`大小`、`加密`、`版本`），
  带失败原因、空态说明与加载态文案。标题行区分完整数量与过滤后数量
  （`共 N 个 tag` / `匹配 M / N 个 tag`），否则部分结果会被误读成全部。
  表格由 `ui.List` + `Rows` 自绘而非用 `ui.Table`：设计稿的选中行是淡紫 tint 加左侧竖条，
  而 `ui.Table` 把选中行硬编码成实心 Accent 块且不留样式钩子。三行网格上那一整块颜色
  的分量远超「用户只是扫一眼」这个状态应有的分量，还会带着格子里每个字的颜色一起走。
- **三态区分**：仓库为空（「此仓库还没有 artifact。」）、被过滤筛空（「没有 tag 匹配「x」。」
  + `清除过滤` 按钮）、列举失败（`重试` 按钮）必须是三段不同文案——前两者需要方向相反的下一步，
  失败态则大多只是临时问题，重试应就地可达。
- **操作区（表格下方）**：`推送…` / `拉取…` / `删除…` / `编辑标签…`，上方一条 1px 分隔线把它们与表格分开（没有这条线，按钮会被读成表格的延续内容）。顺序即此：删除夹在两个可撤销的行操作之间，而不是排在最后——它是这里唯一不可撤销的，贴着「无人区」才让误点不至于丢数据。右侧留一段选中行摘要（`tag · 大小 · 已加密`）：三个按钮的作用域都是这一行，说出来就不用回头去表里核对「它」是谁；摘要是单行（`MaxLines(1)`），换行会把工具栏撑成两行、把按钮挤出矮窗口的底部。推送作用于当前仓库，未选中行时仍可用；其余三个依赖选中行，未选中时该位置替换为提示文案。
- **操作区贴窗口底，不贴表格底**：动作区所在 Column **不加 `Grow`**。上方的网格已经吃掉全部余量，再加一次 Grow 会把它抢回来一半，按钮就停在窗口中间、下面留一大片空白——看着像「内容到此为止」，而拉取/删除/改标签都只能从这里进。保持自然高度才是贴底的机制。
- **模态弹窗**：
  - 拉取——目标路径（带浏览按钮）+ 密码（仅加密产物要求输入）。
  - 推送——源路径（**两个浏览按钮**：`选择文件…` 与 `选择目录…`）、tag、加密开关与密码、
    标签（每行一个 `key=value`）。两个按钮是必需的：MyGo 的打开对话框把文件与目录设为互斥
    （`OpenDialogOptions` 里 `OpenFiles = !Directory`），单个对话框无法同时接受两者，
    否则 GUI 里就根本没法推送目录。表单不完整时确认按钮置灰，错误紧邻出问题的字段显示，而不是等到
    registry 往返失败才报一个不透明的错。
  - 删除确认、失败信息。拉取进行中显示阶段文案与下载进度条。

**两处故意保留英文**：
- `parseLabels` 的错误信息——它与 CLI 共用，归 CLI 所有；GUI 用 `标签有误：%w` 包一层，
  中文句子落在前面。为翻译而在 GUI 里重写校验等于给同一条规则开出第二个事实来源。
- 进度阶段 key（`download` / `decrypt` / `unpack` / `check`）来自 `runPull`；翻译只发生在
  `viewProgress` 的 switch 里。

**全角宽度**：中文是全角字符，视觉宽度约为英文的两倍。表头与固定宽度控件的 `Width` 要留够，
最小窗口（720×420）下的布局回归测试是这一点的守门人。

**文本可读性**（`cmd/gui_contrast.go`）：MyGo 调色板对控件够用，但把次要文字放到 Surface 背景
或选中行上时，有几组配对低于 WCAG AA 正文阈值（4.5:1）。以下是实测值，非估计：

| 前景 / 背景 | light | dark |
|---|---|---|
| `TextMuted` / `Background` | 4.83 | 6.91 |
| `TextMuted` / `Surface` | **4.40** | 5.81 |
| `TextMuted` / `Selection` | **3.37** | **3.88** |
| `Warning` / `Background` | **3.19** | 8.25 |

因此三个辅助函数：`readableSecondary`（Surface 上的次要文字）、
`readableOnSelection`（选中行上的仓库路径）、`readableWarningOn`（表格里的「是」）。
最后一个多带一个背景参数：在白底达标的琥珀色，落到选中行的淡紫 tint 上几乎看不见——
同一行里换个背景就得重算，不重算就是「上一行看得清、这一行看不清」。
`ensureContrast` 在 `Color.Mix` 的混合系数上二分，找到**刚好**达标的最小改动——
既不无脑加深（那会把整个界面压成同一个色调），也不闭式反解 WCAG 公式
（那等于在这里重写一份 Mix，还可能和渲染器算得不一样）。
表单字段标签是「指令」而非装饰，直接用 `th.Text`。

`cmd/gui_contrast_test.go` 锁住这些数字：MyGo 换配色时会红。

## 主题（`cmd/gui_theme.go`）

设计稿的紫色 token 通过 `Context.SetTheme` 落地，但**不整体替换主题**，只覆盖设计系统
真正指定了语义的角色（Accent / AccentHover / AccentPressed / AccentText / Selection /
Focus / Surface / Border）。Danger、Success 与各 pressed 态保持 MyGo 原样——破坏性操作与
成功反馈的对比度是它们自己的控件按自己的前提调过的，整体换掉等于替它们重新拍一次板。
主题在 `view()` 里每帧重算而非启动时算一次，这样系统明暗切换时窗口会跟着走，而不是
卡在打开时的那一侧。

**两个 MyGo 的坑**：

- `ui.Color` 的 alpha 字节为 0 会被当作「没给颜色」而回退到框架默认值。所以 `hex()`
  必须显式写 `A: 0xFF`——否则整个紫色主题会静默渲染成系统蓝，而代码读起来完全正常。
  主题类代码里的字面颜色一律走 `hex()`，不要直接构造 `ui.Color{R:…, G:…, B:…}`。
- `ui.Table` 的选中行硬编码 `Background(t.Accent)` 实心块，且没有公开的行级样式钩子。
  设计稿要的是淡紫 tint + 左侧竖条，因此在 `viewArtifactTable` 里改用
  `ui.List` + `Rows` 自绘行。代价是失去列宽拖拽手柄——一个四列、宽度固定的网格不需要它。

**状态与生命周期约束**（MyGo 特有，写错会静默出错）：

| 约束 | 原因 |
|---|---|
| app state 只存数据、`ui.ListState`、`ui.Handle` | UI 元素在下一次 build pass 即失效，存下来就是陈旧指针 |
| 远端调用一律 `go` + `win.Update(fn)` | `Update` 把结果送回主线程；goroutine 直接改状态会与渲染竞争 |
| `ui.Modal` 的 `open` 必须是 app 字段 | 传局部变量则每帧都是新的 `false`，弹窗永不出现 |
| 用 `jobSeq` 作废过期结果 | 用户切换 shortcut 后，先发的请求返回时不得覆盖新选择 |
| 副作用只在 `Clicked()` 等动作回调里触发 | 渲染代码一次事件可能被执行多次 |
| toolbar 用 `FillWidth()` 而非 `Fill()` | `Fill()` 会占满父容器高度并居中内容，把下方兄弟元素顶出窗口 |
| 弹窗内元素必须逐个验证在视口内 | 弹窗内容变高会把确认按钮推出窗口，`Click` 静默无效；测试断言 `tt.Find(text)` 的 Rect |

**验证**：`cmd/gui_test.go`（36 个用例）用 `ui.NewTester` 做 headless 断言，覆盖过滤、选中、
空态/错误态/加载态、三个弹窗的开关、推送表单校验、进度文案、布局下限与多帧稳定性。约束：

- 除单测外还有一道静态门禁：`go tool mygo vet .`。它在 `go vet` 之外用类型信息检查
  **MyGo build 值的生命周期**——把 `ui.Element` / `ui.Context` / `ui.ListRow` 存进结构体字段、
  包级变量，或传进 goroutine，都会报错。这类 bug 下一帧就失效，肉眼和运行时都看不出来，
  正是 GUI 最需要静态检查的部分。CLI 由 `go.mod` 的 `tool` 指令管理，与 `mygo` 库同版本锁定，
  不要另行 `go install`（那是另一份可能与 `go.mod` 不一致的二进制）；
  升级用 `go get -tool github.com/egoist/mygo/cmd/mygo@vX.Y.Z`。

- 一个 `ListState` 只能被一个 Tester 渲染，第二个 Tester 复用它会 panic，因此「渲染→点击→再断言」
  必须在同一个 tester 内完成。
- 布局断言用 `tt.Find(text)` 的 `Rect`：`h=0` 或 `y + h` 超出窗口高度即视为失败。最小窗口
  （720×420）是布局下限，弹窗内每个字段都要在此尺寸下断言。
  **给弹窗加一行就可能触发它**：加「可推送文件或目录」提示行时，确认按钮被顶到 y=422.6 而溢出。
  提示因此挂在字段标签上（`源路径（文件或目录）`）而不是独占一行。

**未覆盖**：GUI 的 push / pull / delete 打到真实 registry 的端到端路径没有自动化测试——
`internal/oci` 目前只有 `fanOut` 的纯逻辑测试，没有起临时 registry。这三条路径的正确性依赖
CLI 同名命令的等价实现（GUI 直接复用 `runPush` / `runPull` / `runDelete`），以及手工验证。

---

## 7.1 发版

推送 `v*` tag 触发 `.github/workflows/release.yml`，**产物一律由 CI 生成**。完整步骤、
`vendorHash` 失效的排查、容器化 nix 验证的坑，见 `AGENTS.md` 的「发版流程」一节。

版本号出现在三个地方，改一处不够：`internal/version/version.go`、`nix/package.nix`、
本文档头部与 `io.oci-sync.version` 示例。

---

## 8. 后续扩展方向

- **`--insecure`**：支持 HTTP（非 TLS）仓库
- **`--platform`**：多架构 manifest list 支持
- **增量同步**：对比 digest 跳过未变更内容
- **多文件 layer**：多个 layer 对应多个文件，支持细粒度更新
