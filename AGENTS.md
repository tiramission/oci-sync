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
nix build
```

## 📋 开发命令

**构建和测试工作流：**
```bash
go test ./...                    # 运行单元测试
go build ./...                   # 构建所有包
go build -o temps/oci-sync .     # 构建二进制到 temps 目录
nix build                        # 构建 Nix 包（需要 Nix）
nix flake update                 # 更新 flake.lock 依赖
```

**集成测试：**
- 完整运行时检查：`bash temps/run-basic-check.sh`
  - 必须：`OCI_SYNC_TEST_REPO` 环境变量（如 `registry.example.com/test/repo`）
  - 可选：`OCI_SYNC_TEST_TAG_BASE`、`OCI_SYNC_TEST_PASSPHRASE`
  - 测试标准和 `x`（实验性）命令
  - 测试构件自动清理，除非设置 `OCI_SYNC_KEEP_WORKDIR=1`

**临时文件和测试数据：**
- 总是在 `temps/` 目录下创建临时文件和构建产物
- 禁止在项目根目录创建临时文件
- `temps/runtime-check/` 子目录由测试脚本自动创建

## 🔧 代码规范（已在代码库中验证）

**日志：** 只使用 `charm.land/log/v2`。禁止使用标准库 `log` 或 `fmt.Println`（除非用于表格格式化输出，如 cmd/list.go 中的 JSON/YAML）。

**CLI/错误：** 所有命令描述、错误消息和日志必须使用英文（即使需求是中文）。

**安全：** 所有文件解包都使用 `filepath.Abs()` 防止路径穿越攻击（见 `internal/archive/archive.go`）。

**依赖（已锁定）：**
- CLI 框架：`github.com/spf13/cobra`（必需）
- OCI 交互：`oras.land/oras-go/v2`（必需且唯一）
- 日志：`charm.land/log/v2`
- 加密：`golang.org/x/crypto`（scrypt + AES-256-GCM）
- 配置：`gopkg.in/yaml.v3`
- UI：`github.com/pterm/pterm`（表格格式化）

禁止添加新依赖，除非有充分理由。

## 📚 文档更新

**架构或 CLI 参数变更** 必须更新：
- `docs/design.md`（架构、数据流、模块 API）
- `README.md`（用户端示例和工作流）

**功能添加或边界情况** 应尽可能更新 `FEATURE.md`。

## 📁 项目结构

```
cmd/              # CLI 命令（基于 cobra）
internal/
  archive/        # tar.gz 打包/解包
  crypto/         # AES-256-GCM 加密
  oci/            # oras-go v2 push/pull/list/delete
  config/         # YAML 配置解析（gopkg.in/yaml.v3）
nix/              # Nix flake 包和开发 shell
docs/
  design.md       # 完整架构和 API 文档
temps/            # 测试产物和构建输出（git 忽略）
```

## 🎯 关键实现注意事项

- **无 OCI 单元测试**：`internal/oci` 无单元测试（需要实时仓库访问）。仅使用集成测试。
- **配置发现**：自动从 `~/.config/oci-sync/oci-sync.yaml` 或当前目录加载配置。
- **实验性命令**（`x push`、`x pull` 等）：依赖配置文件 `experimental.repo`。
- **清单注解**：加密状态和版本记录在 OCI 镜像清单中，而非文件元数据。
