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

**GUI 文案：** 桌面 GUI（`cmd/gui*.go`）里所有用户可见的字符串一律用**中文**——按钮、表头、
占位符、空态/错误态提示、弹窗标题与字段标签、以及 `validatePush` 的校验错误。
CLI 保持英文，因此这些文案就地写在 GUI 里，不抽成共享文案表：一个错误若要跨两者，
就得选一种语言或者两种都带。

两处例外是**故意保留英文**的，不要「顺手」翻译：
- `cmd/utils.go` 的 `parseLabels` —— 它与 CLI 共用，错误信息归 CLI 所有。GUI 用
  `fmt.Errorf("标签有误：%w", err)` 包一层，中文句子落在前面。为翻译而在 GUI 里重写
  校验逻辑等于给同一条规则开出第二个事实来源。
- 进度阶段 key（`"download"` / `"decrypt"` / `"unpack"` / `"check"`）来自 `runPull`，
  翻译只发生在 `viewProgress` 的 switch 里，不要把这些 key 改成中文往下传。

注意中文是全角字符，视觉宽度是英文的两倍：表头和固定宽度控件的 `Width` 要留够，
新增中文文案后要重跑 `cmd` 里的布局回归测试。

**GUI 文字对比度**：不要直接拿 `th.TextMuted` / `th.Warning` 当正文色。MyGo 调色板里有几组配对
低于 WCAG AA 正文阈值（4.5:1）——`TextMuted`/`Surface` 浅色 4.40、`TextMuted`/`Selection`
浅色 3.37 深色 3.88、`Warning`/`Background` 浅色 3.19。用 `cmd/gui_contrast.go` 里的
`readableSecondary` / `readableOnSelection` / `readableWarning`，它们只在不达标时才动手，
且只改到刚好达标。新增次要文字时同理，并跑 `gui_contrast_test.go`。

**安全：** 所有文件解包都使用 `filepath.Abs()` 防止路径穿越攻击（见 `internal/archive/archive.go`）。

**大文件一律流式：** push/pull 不得把 artifact 整体读进内存。链路是 pack →（加密）→ staging 文件 → 上传，pull 反向，峰值内存是一个 1 MiB 分块（`crypto.ChunkSize`）。中间落地到文件不是偷懒：Registry 按内容 digest 寻址 blob，digest 必须先知道，而刚打好的 tar.gz 还不知道自己的 digest。改动这条链路时，内存版的 `archive.PackBytes`、`oci.Push`、`oci.Pull` 只能留给测试和非流式调用方。

**能推就要能拉：** `archive.Pack` 用 `memberBase` 决定顶层目录名，**不要直接用 `filepath.Base`**——它对 `.` / `..` 返回 `.` / `..`，产出 `./x`、`../x` 这类成员名。写 tar 不拒绝任何名字，所以 push 会成功，而 pull 必然因 `illegal file path in archive` 失败。新增打包路径形态前，先确认 `Pack` → `Unpack` 能往返。

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

**活动历史不要重复记录**：`runPush` / `runPull` / `runDelete` / `runLabelSet` / `runLabelUnset`
内部已经调用 `recordActivity`，GUI 与 TUI 直接复用这些函数即可，不要在界面侧再记一次。

**TUI 多字段表单**：只有聚焦的那一行挂在活的 `textinput` 上，其余行渲染 form 里的字符串。
同时存在多个 textinput 会争抢光标状态。切字段时先把当前行的值写回 form 再 `SetValue` 切走，
否则用户切走时正在编辑的内容会丢。

**口令失焦必须掩码**：`bubbles/textinput` 的 `EchoPassword` 仍然按字符数画点，会泄露口令长度。
失焦行统一画固定长度的掩码符，聚焦行也不画 input 视图（否则用户看不到输入位置）。

**footer 要钳宽度**：`bubbles/help` 的 `ShortHelpView` 会在超过 `help.Width` 时**自行截断**并以省略号收尾。
所以「这行放得下吗」只能问渲染结果里是否还留着每个绑定的描述，量字符串宽度量不出来——
被 help 切过的行量出来反而更短，于是标记会被追加到一条已经切过的尾巴上。
`cmd/tui_view.go` 的 `fitBindings` 从尾部丢绑定直到整行放得下、再补一个指向完整键位表的标记，
`clampHelp` 兜住最后 2 列。少了标记，footer 会在用户以为「就这些键」时静默少掉一半。

**焦点用边框粗细，不用颜色**：终端没有半像素边框，设计稿的 1.5px 只能近似成「双线 vs 单线」。
粗细是余光能捕捉的信号，两个相近紫色之间的换色不是。弹窗/表单接管键盘时两侧都不画焦点。

**MyGo 主题字面量必须走 `hex()`**：`ui.Color` 的 alpha 为 0 会被当作「没给颜色」而回退到框架默认值。
直接构造 `ui.Color{R:…, G:…, B:…}` 会让整套紫色主题静默渲染成系统蓝，而代码读起来完全正常。

**空态必须分三种**：未加载仓库 / 仓库为空 / 被过滤筛空，三者需要的下一步方向不同
（选仓库 / 推送 / 清过滤），混用会给出错误指引。GUI 与 TUI 两侧都要覆盖。

**表格行样式要重新挑色**：选中行换了背景，同一格里每个字的颜色都得按新背景重算。
沿用普通行的 `TextMuted` 或为白底调好的琥珀色，落淡紫 tint 上会几乎看不见。

## 🖼️ UI 改动必须先看真实渲染

**任何 UI 改动，动代码之前先用真实渲染出一张图**：GUI 用
`ui.NewTester(view, w, h).Image()` + `png.Encode`，TUI 直接取 `View()` 的字符串。
凭想象写完再「对照设计稿」是自欺欺人——本项目已经因此返工过一轮：设计稿是淡紫 `#534AB7`，
实际渲染出来是 MyGo 的系统蓝；设计稿有计数角标和常驻状态条，实际一个都没有。
**设计稿是提案，真实渲染才是交付基准**；两者对不上时，先看渲染，再改代码。

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
