# 项目介绍

`oci-sync` 命令行工具，用于将本地文件同步到 OCI 兼容的镜像仓库中，支持单文件和目录，支持加密
使用 go 语言实现

## 主要功能

- `oci-sync push`：将本地文件同步到 OCI 兼容的镜像仓库中。
- `oci-sync pull`：从 OCI 兼容的镜像仓库中同步文件到本地。
- `oci-sync delete`：从 OCI 兼容的镜像仓库中删除文件(镜像)。
- `oci-sync x push`：实验性快捷推送命令，通过配置文件提供 repository，仅用 `--tag` 指定远程标签。
- `oci-sync x pull`：实验性快捷拉取命令，通过配置文件提供 repository，仅用 `--tag` 指定远程标签。
- `oci-sync x list`：实验性快捷列举命令，通过配置文件提供 repository，直接列出所有 tags。
- `oci-sync x delete`：实验性快捷删除命令，通过配置文件提供 repository，仅用 `--tag` 指定删除目标。


### 认证
- 支持配置文件 `auths.<registry>.username/password` 为每个仓库配置独立凭据
- 回退到 Docker credential store（`~/.docker/config.json`），支持 macOS, Windows, Linux


### 实现
尽量不创建轮子，使用现有的库
实现过程，对于文件打包加密压缩，然后创建oci image，最后推送到镜像仓库，拉取过程相反

1. push

```bash
oci-sync push --local <local_path> --remote <remote_path> --passphrase <passphrase>
# 或使用简写
oci-sync push -l <local_path> -r <remote_path> --passphrase <passphrase>
```

- remote 格式为 `<registry>/<repository>:<tag>`
- local 可以是文件或目录
- passphrase 为可选参数，如果提供了 passphrase，则文件会被加密

2. pull

```bash
oci-sync pull --remote <remote_path> --local <local_path> --passphrase <passphrase>
# 或使用简写
oci-sync pull -r <remote_path> -l <local_path> --passphrase <passphrase>
```

- remote 格式为 `<registry>/<repository>:<tag>`
- local 可以是文件或目录
- passphrase 为可选参数，如果提供了 passphrase，则文件会被解密

3. delete

```bash
oci-sync delete --remote <remote_path>
```

- remote_path 格式为 `<registry>/<repository>:<tag>`

4. list

```bash
# 列出特定仓库的 tags
oci-sync list --remote <registry>/<repository>
# 列出整个注册表的所有镜像仓库
oci-sync list --remote <registry>
```

- remote 格式为 `<registry>/<repository>` 或单个 `<registry>`

5. experimental commands

experimental 命令依赖配置文件中的 `experimental.repo`：

```bash
oci-sync x push --local <local_path> --tag <tag> --passphrase <passphrase>
oci-sync x pull --tag <tag> --local <local_path> --passphrase <passphrase>
oci-sync x list
oci-sync x delete --tag <tag>
```

- `--tag` 用于补全远程引用，最终组合为 `<registry>/<repository>:<tag>`
- `--local` 仍然必需；该需求只是简化远程仓库输入，不改变本地文件/目录行为
- `oci-sync x list` 不需要 `--tag`，直接列出该 repository 下的所有 tags
- `oci-sync x delete` 使用 `--tag` 组合出完整远程引用，并删除对应 artifact

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
  another.registry.com:
    username: user2
    password: token2
```

认证优先级：**配置文件 `auths` > Docker credential store**

## Nix 集成

### Home Manager 模块

本项目提供 home-manager 模块，支持通过 Nix 配置管理 oci-sync：

```nix
{
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
```

可用选项：

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enable` | bool | false | 启用 oci-sync |
| `package` | package | - | 自定义包版本 |
| `settings.experimental.enabled` | bool | true | 启用实验性命令 |
| `settings.experimental.repo` | string | "" | 实验性命令默认仓库 |
