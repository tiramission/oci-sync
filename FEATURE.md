# 项目介绍

`oci-sync` 命令行工具，用于将本地文件同步到 OCI 兼容的镜像仓库中，支持单文件和目录，支持加密
使用 go 语言实现

## 主要功能

- `oci-sync push`：将本地文件同步到 OCI 兼容的镜像仓库中。
- `oci-sync pull`：从 OCI 兼容的镜像仓库中同步文件到本地。


### 认证
使用 docker credential store 进行认证，支持 macOS, Windows, Linux


### 实现
尽量不创建轮子，使用现有的库
实现过程，对于文件打包加密压缩，然后创建oci image，最后推送到镜像仓库，拉取过程相反

1. push

```bash
oci-sync push <local_path> <remote_path> --passphrase <passphrase>
```

- remote_path 格式为 `<registry>/<repository>:<tag>`
- local_path 可以是文件或目录
- passphrase 为可选参数，如果提供了 passphrase，则文件会被加密

2. pull

```bash
oci-sync pull <remote_path> <local_path> --passphrase <passphrase>
```

- remote_path 格式为 `<registry>/<repository>:<tag>`
- local_path 可以是文件或目录
- passphrase 为可选参数，如果提供了 passphrase，则文件会被解密