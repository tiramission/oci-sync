# oci-sync AI 编码规范

AI 工具（Cursor、Copilot、OpenCode 等）协助开发此项目时，必须严格遵守以下规范。

## 🚫 Git 工作流（关键）

**⚠️ 禁止自动提交和推送（绝对规则）**

AI 绝对不能自动执行以下操作，除非用户明确、清晰地用中文或英文说"提交"、"push"、"commit"等明确的指令：

- ❌ `git commit`
- ❌ `git push`
- ❌ `git commit --amend`
- ❌ `git push --force`
- ❌ 任何其他强制操作

**工作流程：**
1. AI 进行代码修改后，必须等待用户明确指示才能提交
2. 提交前必须验证所有检查通过（见下文）
3. 如果不确定，必须先询问用户
4. 永远不要假设用户想要提交

**提交前验证（必须全部通过）：**
```bash
go test ./...
go build ./...
git add -A           # nix build 前必须暂存文件
nix build
```

## 📋 开发命令

**构建和测试工作流：**
```bash
go test ./...                    # 运行单元测试
go build ./...                   # 构建所有包
go build -o temps/oci-sync .     # 构建二进制到 temps 目录
git add -A                       # 暂存所有变更（nix build 前必须）
nix build                        # 构建 Nix 包（需要 Nix）
nix flake update                 # 更新 flake.lock 依赖
```

**集成测试：**
- 完整运行时检查：`cd e2e && uv run -m e2e`
  - 必须：`OCI_SYNC_TEST_REPO` 环境变量（如 `registry.example.com/test/repo`）
  - 可选：`OCI_SYNC_TEST_TAG_BASE`、`OCI_SYNC_TEST_PASSPHRASE`
  - 测试 push/list/pull/delete 完整流程
  - 测试构件自动清理，除非设置 `OCI_SYNC_KEEP_WORKDIR=1`

**临时文件和测试数据：**
- Go 构建产物：`temps/` 目录下创建（已在 .gitignore 中）
- e2e 测试产物：`e2e/runtime-check/` 子目录（已在 e2e/.gitignore 中）
- 禁止在项目根目录创建临时文件

## 🔧 代码规范（已在代码库中验证）

**日志：** 只使用 `charm.land/log/v2`。禁止使用标准库 `log` 或 `fmt.Println`（除非用于表格格式化输出，如 cmd/list.go 中的 JSON/YAML）。

**CLI/错误：** 所有命令描述、错误消息和日志必须使用英文（即使需求是中文）。

**安全：** 所有文件解包都使用 `filepath.Abs()` 防止路径穿越攻击（见 `internal/archive/archive.go`）。

**大文件一律流式：** push/pull 不得把 artifact 整体读进内存。链路是 pack →（加密）→ staging 文件 → 上传，pull 反向，峰值内存是一个 1 MiB 分块（`crypto.ChunkSize`）。中间落地到文件不是偷懒：Registry 按内容 digest 寻址 blob，digest 必须先知道，而刚打好的 tar.gz 还不知道自己的 digest。改动这条链路时，内存版的 `archive.PackBytes`、`oci.Push`、`oci.Pull` 只能留给测试和非流式调用方。

**依赖（已锁定）：**
- CLI 框架：`github.com/spf13/cobra`（必需）
- OCI 交互：`oras.land/oras-go/v2`（必需且唯一）
- 日志：`charm.land/log/v2`
- 加密：`golang.org/x/crypto`（scrypt + AES-256-GCM，分块流式）
- 配置：`gopkg.in/yaml.v3`
- UI：`github.com/pterm/pterm`（表格格式化）
- 终端 TUI：`github.com/charmbracelet/bubbletea` + `bubbles` + `lipgloss`
- 桌面 GUI：`github.com/egoist/mygo`（`gui` 子命令，必需）
- **Go 版本：`go.mod` 要求 1.27.1**（mygo v0.4.2 的硬性要求，不可降低）

禁止添加新依赖，除非有充分理由。

## 🖥 GUI 开发（MyGo）

`oci-sync gui` 用 MyGo 原生 UI（package `ui`），与终端 TUI（bubbletea）并存于同一 binary：
`mygo.App.Run()` 在 cobra 的 `RunE` 内调用，窗口关闭后命令返回。

MyGo 的 UI 元素在下一次 build pass 即失效，**不可存入 app state**——只存数据、`ui.Handle` 和控件状态。
副作用必须放在受控的 action 回调里，不能写在渲染代码中（视图一次事件可能重建多次）。

API 以 pinned 源码为准，不要凭记忆：

```bash
go list -m -f '{{.Dir}}' github.com/egoist/mygo
```

**几个容易踩的 API 事实**（都已在 `cmd/gui_app.go` 中按真实签名使用）：

| 想知道 | 实际写法 |
|---|---|
| 颜色 | 没有 `ui.ColorBorder` 这类常量，取 `c.Theme().Border` / `.TextMuted` / `.Danger` / `.Warning` / `.Selection` |
| 次要文字 | 没有 `.Muted()`，用 `.TextColor(c.Theme().TextMuted)` |
| 按钮 | 只有 `Button` / `PrimaryButton` / `ButtonBase`，**没有** `SecondaryButton` / `DangerButton`；危险按钮用 `ButtonBase` 自绘 |
| 样式开关 | `.Bold()`、`.Password()` 无参数；条件样式要分支整个调用（`Element` 只在创建它的调用里注册进 build pass） |
| 事件 | `.Clicked()` / `.Changed()` 是**无参的查询**，回调用 `.OnClick(fn)` / `.OnChange(fn)` |
| 对齐 | 常量是 `ui.Start` / `ui.Center` / `ui.End`，没有 `JustifyEnd` / `AlignTrailing` |
| 句柄 | `ui.Handle{}` 零值可用，没有 `ui.MakeHandle` |
| 列表选择态 | `ui.ListState{Selected: *int}`，指针必须活得比窗口久 |

**布局：`Fill()` 会同时吃掉宽和高。** 在 `Column` 里给某个子元素 `Fill()`，它会占满父容器的高度并把自己的内容居中，从而把后续兄弟元素顶出窗口——表现为按钮 `tt.Find()` 得到 `h=0`、点在视口外。toolbar 这类「高度自适应、只占满宽度」的行要用 `FillWidth()`；真正要吃剩余空间的用 `Grow(1)` + `FillHeight()`。

**验证**：`CGO_ENABLED=0 go test ./...`，UI 行为用 `ui.NewTester` 断言。两个约束：

- 一个 `ListState` 只能被一个 `Tester`（=一个窗口）渲染，第二个 Tester 用同一个会 panic。所以「渲染→点击→再断言」必须在**同一个** tester 里做。
- 布局问题用 `tt.Find(text)` 拿 `Rect` 定位：`h=0` 或 `y + h` 超出窗口高度就说明元素被挤出视口、点不到。最小窗口 720×420 是布局下限，弹窗内每个字段都要在该尺寸下断言。

**活动历史不要重复记录**：`runPush` / `runPull` / `runDelete` 内部已经调用 `recordActivity`，GUI 直接复用这些函数即可，不要在 GUI 侧再记一次。

## 📚 文档更新

**架构或 CLI 参数变更** 必须更新：
- `docs/design.md`（架构、数据流、模块 API）
- `README.md`（用户端示例和工作流）

**功能添加或边界情况** 应尽可能更新 `FEATURE.md`。

## 📁 项目结构

```
cmd/              # CLI 命令（基于 cobra）
internal/
  archive/        # tar.gz 流式打包/解包
  crypto/         # AES-256-GCM 分块流式加密
  oci/            # oras-go v2 push/pull/list/delete
  config/         # YAML 配置解析（gopkg.in/yaml.v3）
nix/              # Nix flake 包和开发 shell
docs/
  design.md       # 完整架构和 API 文档
temps/            # 测试产物和构建输出（git 忽略）
e2e/              # Python e2e 测试项目（独立 uv 项目）
```

## 🎯 关键实现注意事项

- **无 OCI 单元测试**：`internal/oci` 无单元测试（需要实时仓库访问）。仅使用集成测试。
- **配置发现**：自动从 `~/.config/oci-sync/oci-sync.yaml` 或当前目录加载配置。
- **动态命令**（`oci-sync <name> push`、`oci-sync <name> pull` 等）：依赖配置文件 `shortcuts.<name>.repo`。
- **清单注解**：加密状态和版本记录在 OCI 镜像清单中，而非文件元数据。
