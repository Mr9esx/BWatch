# Hozon 字体

所有字体从本地提供，页面无需连接字体 CDN。

| 用途 | 字体 | 配置 | 许可 |
| --- | --- | --- | --- |
| Hozon 字标 | Averia Serif Libre | 700，品牌字符子集（对比方案 13） | AveriaSerifLibre-OFL.txt |
| 中文界面 | 思源黑体 CN 2.005R | 400–600 可变字重 | SourceHanSans-OFL.txt |
| 英文、数字 | IBM Plex Sans | 400–600 可变字重 | IBMPlexSans-OFL.txt |

思源黑体来自 Adobe 官方仓库：
https://github.com/adobe-fonts/source-han-sans/releases/tag/2.005R

中文网页字体经过裁剪，因此按照 OFL 的保留字体名称规则，将其内部名称改为 **Hozon Sans SC**。字形来自思源黑体，字体内保留原作者版权和许可证。

中文字体分为一个常用字包和八个补充包，使用互不重叠的 `unicode-range` 按需加载。常用字包包含 GB2312 一级汉字、当前界面用字、中文标点及假名。补充包保留原始 CN 字体其余字符的覆盖。可变字重限制为页面实际使用的 400–600，避免每个字重重复下载。

英文及数字来自 Google Fonts 官方提供的 IBM Plex Sans WOFF2，保留其原始语言分区和 OFL 许可。
https://github.com/IBM/plex

## 更新

从 `web` 目录运行：

```sh
uv run --with 'fonttools[woff]' scripts/prepare-ui-fonts.py
```

脚本会更新本目录的字体、`ui-fonts.css` 和许可证；不会修改字标字体。更新界面文案后也可重新运行，让新增界面用字进入常用字包。

## 排版

- 正文：14px / 1.65，400。
- 导航、按钮、表单标签：500；选中状态：600。
- 页面标题：24px / 1.4，600；区块标题：16px / 1.5，600。
- 统计数字：26–32px，600；数据、进度及时间使用等宽数字。
- 中文标题不压缩字距；Averia Serif Libre 字标保持 `-.025em` 字距。
