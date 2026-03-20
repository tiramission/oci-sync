# oci-sync AI Coding Guidelines

如果你是协助开发此项目的 AI（如 Cursor、Copilot、Gemini），请严格遵守以下 5 条铁律：

1. **极其精简**：不要随意引入第三方包。CLI 必须使用 `github.com/spf13/cobra`，OCI 交互必须且只能使用 `oras.land/oras-go/v2`。
2. **纯英文输出**：哪怕用户用中文跟你提需求，所有代码中新增的 CLI 提示语、`Short`/`Long` 命令描述、`Error` 包裹信息以及日志打印，必须**全部使用英文**。
3. **安全底线**：在处理文件解包（Unpack）时，必须使用 `filepath.Abs` 校验前缀，严防路径穿越漏洞（Path Traversal）。
4. **日志规范**：统一使用 `github.com/charmbracelet/log` 打印日志，禁止使用标准库的 `log` 或 `fmt.Println`（除非是格式化表格输出）。
5. **文档同步**：任何架构或命令行参数的修改，必须主动同步更新到 `docs/design.md` 和 `README.md`。
6. **需求文档维护**：`FEATURE.md` 原则上是以人类编写输入为主的需求文档，但在你实现拓展功能或解决需求边界后，请尽可能主动帮忙补充更新该文档。
