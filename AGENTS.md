# oci-sync AI 编码规范

AI 工具（Cursor、Copilot、OpenCode 等）协助开发此项目时，必须严格遵守以下规范。

## 🚫 Git 工作流（关键）

**⚠️ 禁止自动提交和推送（绝对规则）**

除非用户明确说"提交"、"push"、"commit"，AI 不得执行 `git commit` / `git push` / `git commit --amend` / `git push --force` 或任何其他不可逆 git 操作。改完代码就停下等指示；不确定就先问，永远不要假设用户想提交。

**提交前验证（必须全部通过）：**
```bash
CGO_ENABLED=0 go build ./...
go vet ./...
go tool mygo vet .                                   # MyGo build 值生命周期，见 GUI 章节
CGO_ENABLED=0 RUNEWIDTH_EASTASIAN=0 go test -count=1 ./...   # TUI 布局随 locale 变，两种都要跑
CGO_ENABLED=0 RUNEWIDTH_EASTASIAN=1 go test -count=1 ./...
git add -A                                           # nix build 前必须暂存文件
nix build
```

## 🚀 发版流程

推送 `v*` tag 即触发 `.github/workflows/release.yml`：前置验证 → 交叉编译 5 个平台 → 建 GitHub Release。**发布产物由 CI 生成，本地不手动构建上传。**

```bash
# 1. 改版本号（三个文件，缺一不可）
#    internal/version/version.go   var Version = "0.8.0"       # 无 v 前缀
#    nix/package.nix               version = "0.8.0";          # vendorHash 见下
#    docs/design.md                头部版本行 + io.oci-sync.version 示例

# 2. 验证：提交前验证那一套（build / vet / mygo vet / 双 locale 测试）

# 3. 提交、推送、打 annotated tag
git commit -m "chore: bump version to 0.8.0"
git push origin main
git tag -a v0.8.0 -m "版本说明"
git push origin v0.8.0

# 4. 验证自动构建
gh run list --limit 1
gh run watch <run-id> --exit-status

# 5. 重写发布文档（自动生成的只有一行 Full Changelog 链接）
gh release edit v0.8.0 --notes-file notes.md --title "v0.8.0 — <一句话卖点>"
```

**版本号不能带 `v` 前缀**：CI 把 tag 去掉 `v` 后注入 ldflags，该字符串同时写入 `io.oci-sync.version` annotation 随产物进入 registry——带前缀会让同一版本在不同来源下不一致。CI 会断言产物 `--version` 输出，本地这关跑不过就别发。

**`nix/package.nix` 的 `vendorHash` 会因依赖图变化而失效**：症状 `error: hash mismatch in fixed-output derivation`，把报错里的 `got:` 值填回去。注意**命中 nix 缓存时这个错误不暴露**——只有版本号或依赖真正变化触发重编译时才看得见。

**发版前验证（本机无 nix 时用容器）：**
```bash
# PortableGit 会把 -v .:/x 和 -w /x 转成相对路径，必须关掉路径转换并给绝对路径
MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' wslc run --rm \
  -v "$(pwd -W)":/oci-sync -w /oci-sync -v nix:/nix \
  nixos/nix nix --extra-experimental-features nix-command \
  --extra-experimental-features flakes build --no-link --print-build-logs
```
`nixos/nix` 镜像是 musl，**跑不了 glibc 动态链接的 Linux 产物**——要实机运行产物得用 `ubuntu:24.04`。输出先落盘再读全文，别 `tail` 几行就下结论。

**产物验证**：至少实机跑一个平台的 `--version` 确认版本号（Windows 可直接跑；Linux 用 `ubuntu:24.04` 容器）。macOS 产物只能验证可编译，**能否开窗要靠人工在 Mac 上确认**。

**`gh release edit` 是发布文档的唯一来源**：CI 的 `generate_release_notes` 只产出一行对比链接，中文的「新增 / 修复 / 工程 / 安装」四节要手写。

## 📋 开发命令

提交前验证见上。常用补充命令：

```bash
go build -o temps/oci-sync .     # 构建二进制到 temps 目录
nix flake update                 # 更新 flake.lock 依赖
```

- **集成测试**：`cd e2e && uv run -m e2e`。必须设 `OCI_SYNC_TEST_REPO`（如 `registry.example.com/test/repo`）；可选 `OCI_SYNC_TEST_TAG_BASE`、`OCI_SYNC_TEST_PASSPHRASE`。设 `OCI_SYNC_KEEP_WORKDIR=1` 可保留构件。
- **临时文件**：Go 构建产物放 `temps/`，e2e 产物放 `e2e/runtime-check/`（均已 gitignore）。禁止在项目根目录创建临时文件。

## 🔧 代码规范

**日志**：只用 `charm.land/log/v2`。禁用标准库 `log` 和 `fmt.Println`（表格格式化输出除外，如 `cmd/list.go` 的 JSON/YAML）。

**CLI/错误**：命令描述、错误消息、日志一律英文（即使需求是中文）。

**GUI 文案**：桌面 GUI（`cmd/gui*.go`）用户可见字符串一律中文——按钮、表头、占位符、空态/错误态提示、弹窗标题与字段标签、`validatePush` 的校验错误。不抽共享文案表：一个错误若要跨 CLI/GUI 两种语言，就地写在 GUI 里。

两处**故意保留英文**，不要「顺手」翻译：
- `cmd/utils.go` 的 `parseLabels` 与 CLI 共用，错误信息归 CLI 所有；GUI 用 `fmt.Errorf("标签有误：%w", err)` 包一层，中文句子落在前面。为翻译而在 GUI 里重写校验逻辑，等于给同一条规则开出第二个事实来源。
- 进度阶段 key（`"download"` / `"decrypt"` / `"unpack"` / `"check"`）来自 `runPull`，翻译只发生在 `viewProgress` 的 switch 里，不要把 key 改成中文往下传。

中文是全角字符，视觉宽度是英文的两倍：表头和固定宽度控件的 `Width` 要留够，新增中文文案后要重跑 `cmd` 里的布局回归测试。

**GUI 文字对比度**：不要把 `th.TextMuted` / `th.Warning` 直接当正文色。MyGo 调色板里有几组配对低于 WCAG AA 正文阈值（4.5:1）——`TextMuted`/`Surface` 浅色 4.40、`TextMuted`/`Selection` 浅色 3.37 深色 3.88、`Warning`/`Background` 浅色 3.19。用 `cmd/gui_contrast.go` 里的 `readableSecondary` / `readableOnSelection` / `readableWarning`，它们只在不达标时才动手，且只改到刚好达标。新增次要文字时同理，并跑 `gui_contrast_test.go`。

**安全**：所有文件解包都使用 `filepath.Abs()` 防止路径穿越攻击（见 `internal/archive/archive.go`）。

**大文件一律流式**：push/pull 不得把 artifact 整体读进内存。链路是 pack →（加密）→ staging 文件 → 上传，pull 反向，峰值内存是一个 1 MiB 分块（`crypto.ChunkSize`）。中间落地到文件不是偷懒：Registry 按内容 digest 寻址 blob，digest 必须先知道，而刚打好的 tar.gz 还不知道自己的 digest。改动这条链路时，内存版的 `archive.PackBytes`、`oci.Push`、`oci.Pull` 只能留给测试和非流式调用方。

**能推就要能拉**：`archive.Pack` 用 `memberBase` 决定顶层目录名，**不要直接用 `filepath.Base`**——它对 `.` / `..` 返回 `.` / `..`，产出 `./x`、`../x` 这类成员名。写 tar 不拒绝任何名字，所以 push 会成功，而 pull 必然因 `illegal file path in archive` 失败。新增打包路径形态前，先确认 `Pack` → `Unpack` 能往返。

**原子替换要重试**：Windows 上实时扫描器会短暂持有刚写出的目标文件，`os.Rename` 覆盖期间失败（本机失败率约 50%，活动记录会静默丢失）。新写 tmp→rename 路径时复用 `internal/cache` 的 `renameWithRetry`，不要裸调 `os.Rename`。

**依赖（已锁定）**：`spf13/cobra`（CLI）、`oras.land/oras-go/v2`（OCI 交互，必需且唯一）、`charm.land/log/v2`（日志）、`golang.org/x/crypto`（scrypt + AES-256-GCM，分块流式）、`gopkg.in/yaml.v3`（配置）、`pterm`（表格格式化）、`charmbracelet/bubbletea` + `bubbles` + `lipgloss`（TUI）、`egoist/mygo`（GUI，`gui` 子命令必需）。**Go 版本 `go.mod` 要求 1.27.1**（mygo v0.4.2 的硬性要求，不可降低）。

禁止添加新依赖，除非有充分理由。

## 🖥 GUI 开发（MyGo）

`oci-sync gui` 用 MyGo 原生 UI（package `ui`），与终端 TUI（bubbletea）并存于同一 binary：`mygo.App.Run()` 在 cobra 的 `RunE` 内调用，窗口关闭后命令返回。

**MyGo 的 UI 元素在下一次 build pass 即失效，不可存入 app state**——只存数据、`ui.Handle` 和控件状态。副作用必须放在受控的 action 回调里，不能写在渲染代码中（视图一次事件可能重建多次）。`go tool mygo vet .` 除了跑 `go vet`，还用类型信息检查这条生命周期——把 `ui.Element` / `ui.Context` / `ui.ListRow` 等存进结构体字段、包级变量，或传进 goroutine，都会报错；这些正是 GUI 里最难靠肉眼发现的一类 bug（下一帧就失效），所以是硬门禁。

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

**MyGo 主题字面量必须走 `hex()`**：`ui.Color` 的 alpha 为 0 会被当作「没给颜色」而回退到框架默认值。直接构造 `ui.Color{R:…, G:…, B:…}` 会让整套紫色主题静默渲染成系统蓝，而代码读起来完全正常。

**表格行样式要重新挑色**：选中行换了背景，同一格里每个字的颜色都得按新背景重算。沿用普通行的 `TextMuted` 或为白底调好的琥珀色，落淡紫 tint 上会几乎看不见。

**验证**：`go tool mygo vet .` 与 `CGO_ENABLED=0 go test ./...`，UI 行为用 `ui.NewTester` 断言。两个约束：

- 一个 `ListState` 只能被一个 `Tester`（=一个窗口）渲染，第二个 Tester 用同一个会 panic。所以「渲染→点击→再断言」必须在**同一个** tester 里做。
- 布局问题用 `tt.Find(text)` 拿 `Rect` 定位：`h=0` 或 `y + h` 超出窗口高度就说明元素被挤出视口、点不到。最小窗口 720×420 是布局下限，弹窗内每个字段都要在该尺寸下断言。

**活动历史不要重复记录**：`runPush` / `runPull` / `runDelete` / `runLabelSet` / `runLabelUnset` 内部已经调用 `recordActivity`，GUI 与 TUI 直接复用这些函数即可，不要在界面侧再记一次。

mygo CLI 由 `go.mod` 的 `tool` 指令管理，`go tool mygo vet .` 即可，版本与 `github.com/egoist/mygo` 一起锁定，**不要**再 `go install` 到 `GOBIN`——那样装出来的是另一份可能与 `go.mod` 不一致的二进制。升级用 `go get -tool github.com/egoist/mygo/cmd/mygo@vX.Y.Z`（会同时更新库依赖与工具）。

## ⌨️ TUI（bubbletea）

**布局改动必须跑两遍 locale**：`RUNEWIDTH_EASTASIAN=0` 与 `=1` 各一次。终端宽度随 locale 变化，只在一台机器上验证等于没验证——本项目已经因此出现过一次「本地全绿、用户机器失败」的 footer 缺陷。

**宽度只能用 `cellWidth`/`cellCond`，不要用 `lipgloss.Width` 或 `runewidth`**：东亚「歧义宽度」字符（`↑` `↓` `•` `→` `…`）在西文终端算 1 列、在 CJK 终端算 2 列，而 `lipgloss.Width` 和 `runewidth.DefaultCondition` 都**跟着 locale 走**。于是同一个 footer 在 help 里量成 90 列、在别处量成 100 列：要么撑破窗口让终端滚动，要么被截成 `? …` 把所有键位提示全丢掉，且在两种机器上表现不一样。`cmd/tui.go` 的 `cellCond` 固定按 1 列算，`cellWidth` 会自己剥 ANSI；`place`/`joinVertical` 也是自己实现而非 `lipgloss.Place`/`JoinVertical`——组合器也必须用同一把尺，否则前面每段的精确计算会在最后一步被推翻。

**footer 的宽度只在这里判一次**：`bubbles/help` 的 `ShortHelpView` 会按 `help.Width` 自行截断，而它内部的宽度是 locale 相关的。所以走 `untruncatingHelp(h)`（`Width = 0`）拿不截断的渲染，由 `fitBindings` 用 `cellWidth` 从尾部丢绑定，**丢过东西才补 `? more` 指针**——宽度够放下全部键位时指针是多余的（它指向的键位表就在屏幕上）。`clampHelp` 只作兜底，正常情况下是 no-op。少了指针，footer 会在用户以为「就这些键」时静默少掉一半。

**渲染高度必须等于窗口高度**：气泡/边框的 `Height()` 只算**内容**，但面板自己还画上下两条边框，而 `View()` 外面又拼了 header 1 行 + footer 1 行。这四行不在窗口的高度预算里，漏算的表现是终端整体滚动——header 滚出顶部、footer 被切一半，看起来像渲染坏了而不是像超了两行。`cmd/tui_view.go` 的 `paneHeight()` 统一扣掉这四行。**宽度断言查不出超高**：超高的每一行仍然在宽度限制内，只量宽度的尺寸测试会一路绿灯，所以改布局后必须断言渲染结果的**行数**等于窗口高度（`TestRenderedRowsMatchTheWindowHeight`）。

**弹窗要限高，且 hint 最后才丢**：lipgloss 的 `MaxHeight` 卡的是**整个框含边框**，而 `Padding` 在 `MaxHeight` **之前**生效，所以正文可用高度 = 窗口 − 2 边框 − 2 padding。用 `Height()` 会把矮弹窗撑到满窗，留下一个大空框；从尾部裁正文，`enter/esc: close` 这类提示行永远保留——它写着怎么关闭，丢了就没路了。测试必须用**窄宽度**（48 列）跑：80 列时最宽的弹窗本来就放得下，无论实现怎么写都会通过。

**字段标签要留间隙**：标签列按最长键 +1 补齐。`padRight` 在字符串宽度**正好等于**目标宽度时不补空格，而 `repository` 正好 10 字符——按 10 补齐会让详情页第一个字段的键和值粘在一起。

**多字段表单**：只有聚焦的那一行挂在活的 `textinput` 上，其余行渲染 form 里的字符串。同时存在多个 textinput 会争抢光标状态。切字段时先把当前行的值写回 form 再 `SetValue` 切走，否则用户切走时正在编辑的内容会丢。

**口令失焦必须掩码**：`bubbles/textinput` 的 `EchoPassword` 仍然按字符数画点，会泄露口令长度。失焦行统一画固定长度的掩码符，聚焦行也不画 input 视图（否则用户看不到输入位置）。

**焦点用边框粗细，不用颜色**：终端没有半像素边框，设计稿的 1.5px 只能近似成「双线 vs 单线」。粗细是余光能捕捉的信号，两个相近紫色之间的换色不是。弹窗/表单接管键盘时两侧都不画焦点。

**空态必须分三种**：未加载仓库 / 仓库为空 / 被过滤筛空，三者需要的下一步方向不同（选仓库 / 推送 / 清过滤），混用会给出错误指引。GUI 与 TUI 两侧都要覆盖。

## 🖼️ UI 改动必须先看真实渲染

**任何 UI 改动，动代码之前先用真实渲染出一张图**：GUI 用 `ui.NewTester(view, w, h).Image()` + `png.Encode`，TUI 直接取 `View()` 的字符串。凭想象写完再「对照设计稿」是自欺欺人——本项目已经因此返工过一轮：设计稿是淡紫 `#534AB7`，实际渲染出来是 MyGo 的系统蓝；设计稿有计数角标和常驻状态条，实际一个都没有。**设计稿是提案，真实渲染才是交付基准**；两者对不上时，先看渲染，再改代码。

## 📚 文档更新

- `docs/design.md`：架构、数据流、模块 API；**TUI/GUI 交互设计与实现细节的唯一文档来源**（§7）
- `README.md`：用户用法、示例、工作流
- `FEATURE.md`：需求层——功能列表、认证、配置 schema、Nix 模块；不抄用法

架构 / CLI 参数 / UI 行为变更必须同步上述对应文档。键位表的唯一事实来源是 `cmd/tui_keys.go` + design.md §7，不要在 README 里另抄一份。

## 📁 项目结构

完整目录树见 `docs/design.md` §3。要点：`cmd/` 是 cobra 命令与 TUI/GUI 的合体（三者共用 `stage.go` 流式 staging 链与 `utils.go`）；`internal/` 按职责分 archive / crypto / oci / cache / config / xdg / version；`e2e/` 是独立的 Python uv 项目；`nix/` 是 flake 包与开发 shell。

## 🎯 关键实现注意事项

- **无 OCI 单元测试**：`internal/oci` 无单元测试（需要实时仓库访问）。仅使用集成测试。
- **配置发现**：自动从 `./oci-sync.yaml` 或 `~/.config/oci-sync/oci-sync.yaml` 加载配置。动态命令（`oci-sync <name> push/pull/list/delete`）依赖配置文件 `shortcuts.<name>.repo`。
- **清单注解**：加密状态（`io.oci-sync.encrypted`）和版本（`io.oci-sync.version`）记录在 OCI 镜像清单的 annotations 中，而非文件元数据；`list` 靠 version 注解识别本工具上传的产物。
