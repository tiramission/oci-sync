# oci-sync

将本地文件或目录同步到 OCI 兼容的镜像仓库中。支持文件/目录、可选加密，支持配置文件凭据和 Docker credential store 认证。

## 安装

### 使用 Go
```bash
go install github.com/tiramission/oci-sync@latest
```

或从源码构建：
```bash
git clone https://github.com/tiramission/oci-sync.git
cd oci-sync
go build -o oci-sync .
```

### 使用 Nix (Flake)
本项目已提供 `flake.nix`，Nix 用户可直接运行：
```bash
nix run github:tiramission/oci-sync -- --help
```

开发者也可直接进入开发环境：
```bash
nix develop github:tiramission/oci-sync
```

### 使用 Home Manager
在 `flake.nix` 中引用 home-manager 模块：

```nix
{
  inputs.home-manager.url = "github:nix-community/home-manager";
  inputs.oci-sync.url = "github:tiramission/oci-sync";

  outputs = { self, nixpkgs, home-manager, oci-sync }: {
    homeConfigurations.myuser = home-manager.lib.homeManagerConfiguration {
      modules = [
        home-manager.nixosModules.home-manager
        {
          home.username = "myuser";
          home.homeDirectory = "/home/myuser";
          programs.oci-sync = {
            enable = true;
            settings = {
              shortcuts = {
                x.repo = "registry.example.com/myteam/files";
              };
            };
          };
        }
        oci-sync.homeModules.oci-sync
      ];
    };
  };
}
```

## 前置条件

通过配置文件或 `docker login` 登录目标仓库。

## 使用

### push — 推送到仓库

```bash
# 推送目录（不加密）
oci-sync push --local ./mydir --remote registry.example.com/myrepo:latest

# 推送文件（加密）
oci-sync push --local ./secret.txt --remote registry.example.com/myrepo:encrypted --passphrase mypassword

# 使用简写标志
oci-sync push -l ./mydir -r registry.example.com/myrepo:latest
```

### pull — 从仓库拉取

```bash
# 拉取（不加密）
oci-sync pull --remote registry.example.com/myrepo:latest --local ./output

# 拉取并解密
oci-sync pull --remote registry.example.com/myrepo:encrypted --local ./output --passphrase mypassword

# 使用简写标志
oci-sync pull -r registry.example.com/myrepo:latest -l ./output
```

### <name> push / pull / list / delete — 动态快捷命令

动态命令通过配置文件 `shortcuts.<name>.repo` 定义，只需通过 `--tag` 指定标签：

```bash
# 推送目录
oci-sync x push --local ./mydir --tag latest

# 推送文件并加密
oci-sync x push --local ./secret.txt --tag encrypted --passphrase mypassword

# 拉取到本地目录
oci-sync x pull --tag latest --local ./output

# 拉取并解密
oci-sync x pull --tag encrypted --local ./output --passphrase mypassword

# 列出快捷仓库下的所有 tags
oci-sync x list

# 以 JSON 格式输出
oci-sync x list --format json

# 筛选包含特定标签的镜像
oci-sync x list --label app=myapp

# 删除快捷仓库中的指定 tag
oci-sync x delete --tag old-release
```

### delete — 删除仓库中的文件

```bash
# 从远程仓库删除推送的文件
oci-sync delete --remote registry.example.com/myrepo:latest

# 使用简写标志
oci-sync delete -r registry.example.com/myrepo:latest
```

### list — 列出仓库中的文件镜像

```bash
# 检索远程仓库中特定路径的文件镜像
oci-sync list --remote registry.example.com/myrepo

# 检索整个注册表下的所有由本工具上传的镜像记录
oci-sync list -r registry.example.com

# 以 JSON 格式输出
oci-sync list -r registry.example.com/myrepo --format json

# 以 YAML 格式输出
oci-sync list -r registry.example.com/myrepo -f yaml

# 筛选包含特定标签的镜像
oci-sync list -r registry.example.com/myrepo --label app=myapp

# 筛选包含特定标签 key 的镜像
oci-sync list -r registry.example.com/myrepo --label env
```

### label — 管理标签

```bash
# 设置标签
oci-sync label set --remote registry.example.com/myrepo:tag key1=value1 key2=value2

# 设置空值标签
oci-sync label set --remote registry.example.com/myrepo:tag app=

# 删除标签
oci-sync label unset --remote registry.example.com/myrepo:tag key1 key2
```

### alias — 管理 shortcuts

```bash
# 列出所有 shortcuts
oci-sync alias list

# 添加 shortcut
oci-sync alias add x --repo registry.example.com/myteam/files

# 删除 shortcut
oci-sync alias remove x
```

### recent — 查看活动历史

查看 push/pull/delete/label 等操作的历史记录（存储在本地 cache）。

```bash
# 查看最近活动（默认 20 条）
oci-sync recent

# 指定数量
oci-sync recent --limit 10

# 指定格式
oci-sync recent --format json
oci-sync recent --format yaml

# 清空历史记录
oci-sync recent --clear
```

### tui — 交互式终端界面

启动全屏交互式 TUI 来管理 shortcuts 和 artifacts：

```bash
oci-sync tui
```

**功能特性**

- 📋 **浏览 Shortcuts**：选择并浏览所有配置的 shortcuts
- 📦 **查看 Artifacts**：列出每个 shortcut 仓库中的所有 artifacts，显示加密状态和标签
- 🔍 **详细信息**：查看每个 artifact 的完整信息（摘要、加密状态、版本、标签等）
- 📤 **上传**：交互式分步上传（输入路径→指定标签→可选加密）
- ⬇️ **下载**：交互式分步下载（选择目标位置→输入密码，自动解密）
- 🗑️ **删除**：删除 artifacts（带确认提示）
- 🔄 **刷新**：实时更新 artifacts 列表

**快捷键**

| 快捷键 | 功能 | 场景 |
|------|------|------|
| `↑/↓` 或 `k/j` | 导航 | 所有列表 |
| `Enter` | 进入/选择 | 列表、表单 |
| `u` | 上传 | Artifacts 列表 |
| `d` | 下载 | Artifacts 列表 |
| `x` | 删除 | Artifacts 列表 |
| `enter` | 查看/提交 | Artifacts 列表、表单 |
| `Esc` | 取消 | 模态对话框 |
| `y/n` | 确认/取消 | 确认对话框 |
| `b` | 返回 | Artifacts/详情 |
| `r` | 刷新 | Artifacts 列表 |
| `q` | 退出 | 全局 |

**密码输入**

上传/下载时的密码输入会自动屏蔽显示（`•`），确保隐私安全。

### 参数说明

| 参数 | 说明 |
|------|------|
| `--local`, `-l` | 本地文件或目录路径（push）或目标目录（pull）|
| `--remote`, `-r` | OCI 仓库引用 (push/pull/delete) 或注册表引用 (list) |
| `--tag` | 快捷命令 `x push / x pull / x delete` 使用的标签 |
| `--passphrase`| 加密/解密口令（可选） |
| `--format`, `-f` | 输出格式：`table`（默认）、`json`、`yaml` |
| `--label` | 设置/筛选标签（格式：`key=value`，value 可为空；仅 `key` 时检查 key 是否存在） |
| `--quiet`, `-q` | 开启静默模式，仅输出错误信息 |

### 配置文件

配置文件使用 YAML 格式，搜索路径如下：

1. 当前工作目录 `./oci-sync.yaml`
2. 用户配置目录 `~/.config/oci-sync/oci-sync.yaml`

配置文件格式：

```yaml
shortcuts:
  x:
    repo: registry.example.com/myteam/files

auths:
  registry.example.com:
    username: myuser
    password: mytoken

experiments:
  tui: false  # 启用实验性 TUI 功能（也可通过 OCI_SYNC_TUI=1 环境变量开启）
```

可用配置项：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `shortcuts.<name>.repo` | - | 动态命令的默认仓库地址 |
| `auths.<registry>.username` | - | 该仓库的认证用户名 |
| `auths.<registry>.password` | - | 该仓库的认证密码或令牌 |
| `experiments.tui` | `false` | 启用实验性 TUI 功能（也可通过 `OCI_SYNC_TUI=1` 环境变量开启） |

认证优先级：**配置文件 `auths` > Docker credential store**

## 工作原理

**push**：本地文件/目录 → tar.gz 打包 → [可选] AES-256-GCM 加密 → 推送至 OCI 仓库

**pull**：从 OCI 仓库检查加密状态 → 校验密码参数（若缺失则快速失败）→ 拉取数据 → [可选] 解密 → 解压 tar.gz → 写入本地

加密使用 scrypt 从口令派生密钥（N=32768），每次加密使用随机 salt 和 nonce，安全可靠。

认证支持配置文件 per-registry 凭据（`auths.<registry>`），也兼容 Docker credential store（`~/.docker/config.json`）。

## 详细设计

见 [docs/design.md](docs/design.md)。
