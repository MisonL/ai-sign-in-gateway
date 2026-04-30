# Wiki 发布说明

本目录按 GitHub Wiki 的常见约定组织：

- `Home.md` 是 Wiki 首页。
- `_Sidebar.md` 是侧边栏。
- `_Footer.md` 是页脚。
- 其它页面使用 ASCII 文件名、连字符和 `.md` 扩展名，便于跨平台维护。
- 页面内容使用 GitHub Markdown。

GitHub Wiki 是独立 Git 仓库，地址通常为：

```text
git@github.com:vikingleo/ai-sign-in-gateway.wiki.git
```

主仓库中的 `wiki/` 目录不会自动发布到 GitHub Wiki，需要同步到独立 wiki 仓库。

## 使用同步脚本

在主仓库根目录运行：

```bash
./scripts/sync-wiki.sh
```

脚本默认行为：

| 项 | 默认值 |
|---|---|
| 源目录 | `<repo>/wiki` |
| Wiki remote | 由 `origin` 推导为 `*.wiki.git` |
| 本地 wiki 工作目录 | `<repo>/../ai-sign-in-gateway.wiki` |
| Commit message | `docs: update wiki` |

可覆盖：

```bash
WIKI_REMOTE=git@github.com:vikingleo/ai-sign-in-gateway.wiki.git \
WIKI_WORKTREE=../ai-sign-in-gateway.wiki \
WIKI_COMMIT_MESSAGE="docs: initialize project wiki" \
./scripts/sync-wiki.sh
```

## 手动同步

```bash
git clone git@github.com:vikingleo/ai-sign-in-gateway.wiki.git ../ai-sign-in-gateway.wiki
rsync -av --delete --exclude='.git' wiki/ ../ai-sign-in-gateway.wiki/
cd ../ai-sign-in-gateway.wiki
git add -A
git commit -m "docs: initialize project wiki"
git push
```

如果 wiki 仓库还不能 clone，先在 GitHub 仓库页面确认 Wiki 功能已启用，并通过网页创建第一版 Wiki 页面。

## 官方参考

- [GitHub Docs: About wikis](https://docs.github.com/en/communities/documenting-your-project-with-wikis/about-wikis)
- [GitHub Docs: Adding or editing wiki pages](https://docs.github.com/en/communities/documenting-your-project-with-wikis/adding-or-editing-wiki-pages)
- [GitHub Docs: Editing wiki content](https://docs.github.com/en/communities/documenting-your-project-with-wikis/editing-wiki-content)
- [GitHub Docs: Creating a footer or sidebar for your wiki](https://docs.github.com/en/communities/documenting-your-project-with-wikis/creating-a-footer-or-sidebar-for-your-wiki)
- [GitHub Docs: Basic writing and formatting syntax](https://docs.github.com/en/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax)
