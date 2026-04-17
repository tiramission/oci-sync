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
              experimental = {
                enabled = true;
                repo = "registry.example.com/myteam/files";
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

### x push / x pull / x list / x delete — 实验性快捷命令

实验性命令依赖配置文件中的 `experimental.repo`，只需通过 `--tag` 指定标签：

```bash
# 推送目录
oci-sync x push --local ./mydir --tag latest

# 推送文件并加密
oci-sync x push --local ./secret.txt --tag encrypted --passphrase mypassword

# 拉取到本地目录
oci-sync x pull --tag latest --local ./output

# 拉取并解密
oci-sync x pull --tag encrypted --local ./output --passphrase mypassword

# 列出实验性仓库下的所有 tags
oci-sync x list

# 以 JSON 格式输出
oci-sync x list --format json

# 删除实验性仓库中的指定 tag
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
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--local`, `-l` | 本地文件或目录路径（push）或目标目录（pull）|
| `--remote`, `-r` | OCI 仓库引用 (push/pull/delete) 或注册表引用 (list) |
| `--tag` | 实验性 `x push` / `x pull` / `x delete` 使用的标签 |
| `--passphrase`| 加密/解密口令（可选） |
| `--format`, `-f` | 输出格式：`table`（默认）、`json`、`yaml` |
| `--quiet`, `-q` | 开启静默模式，仅输出错误信息 |

### 配置文件

配置文件使用 YAML 格式，搜索路径如下：

1. 当前工作目录 `./oci-sync.yaml`
2. 用户配置目录 `~/.config/oci-sync/oci-sync.yaml`

配置文件格式：

```yaml
experimental:
  enabled: true
  repo: registry.example.com/myteam/files

auths:
  registry.example.com:
    username: myuser
    password: mytoken
```

可用配置项：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `experimental.enabled` | `true` | 是否启用实验性命令 |
| `experimental.repo` | - | 实验性命令使用的仓库地址 |
| `auths.<registry>.username` | - | 该仓库的认证用户名 |
| `auths.<registry>.password` | - | 该仓库的认证密码或令牌 |

认证优先级：**配置文件 `auths` > Docker credential store**

## 工作原理

**push**：本地文件/目录 → tar.gz 打包 → [可选] AES-256-GCM 加密 → 推送至 OCI 仓库

**pull**：从 OCI 仓库检查加密状态 → 校验密码参数（若缺失则快速失败）→ 拉取数据 → [可选] 解密 → 解压 tar.gz → 写入本地

加密使用 scrypt 从口令派生密钥（N=32768），每次加密使用随机 salt 和 nonce，安全可靠。

认证支持配置文件 per-registry 凭据（`auths.<registry>`），也兼容 Docker credential store（`~/.docker/config.json`）。

## 详细设计

见 [docs/design.md](docs/design.md)。
