# Hozon 品牌素材

Hozon 取自日语「保存」，用于内容订阅与本地归档产品。

## 正式页面字标

已选定对比方案 **13 · Averia Serif Libre 700**；字距为 `-.025em`，桌面 25px、手机 21px。字体只用于 Hozon 字标，页面文字继续使用界面字体。

正式字体保存在 `../../web/public/fonts/averia-serif-libre-hozon-700.ttf`，与应用一起分发并预加载。完整版权与 OFL 许可证保存在同目录的 `AveriaSerifLibre-OFL.txt`。下方 v1 横向图片是最初的设计稿。

## 页面字体

中文界面使用思源黑体 CN 2.005R 的本地可变字体，英文和数字使用 IBM Plex Sans 400–600。中文字体裁剪后按 OFL 要求将内部家族名称改为 Hozon Sans SC，字形与来源保留记录。页面默认正文 400，导航及按钮 500，标题及重点数据 600。

字体分包、授权和更新方法见 `../../web/public/fonts/README.md`。

## 素材与对比

- `hozon-logo-concept-v1.png`：获认可的透明底横向组合。
- `hozon-logo-preview-v1.png`：浅色背景展示稿。
- `../../web/public/hozon-mark.png`：用于应用导航与 favicon 的独立标记。
- `../../web/hozon-type-comparison.html`：十三款字标对比，包含侧栏和手机顶栏尺寸，字体及图形已内嵌，可离线打开。当前优先展示 Fraunces 与新增五款衬线字体，其他七款可展开比较。开发预览地址为 `/hozon-type-comparison.html`。

字体来自 Google Fonts 官方 CSS API，使用仅包含 `Hozon` 字符的字体子集，原始字体文件及 OFL 许可证保存在 `fonts/`。对比页不作为正式应用的构建入口。

| 编号 | 字体 | 字重 | 方向 |
| --- | --- | --- | --- |
| 01 | Outfit | 600 | 几何、圆润 |
| 02 | Space Grotesk | 600 | 利落、工具感 |
| 03 | Fraunces | 600（opsz 24） | 柔和衬线 |
| 04 | Fredoka | 600 | 圆润、轻松 |
| 05 | Baloo 2 | 600 | 饱满、俏皮 |
| 06 | Righteous | 400 | 复古、几何 |
| 07 | Grandstander | 600 | 手写、活泼 |
| 08 | Pacifico | 400 | 连笔、随性 |
| 09 | Young Serif | 400 | 柔和、旧书感 |
| 10 | Caprasimo | 400 | 圆厚、复古；基于 Fraunces |
| 11 | Bree Serif | 400 | 温和、轻快 |
| 12 | DM Serif Display | 400 | 书刊、精致 |
| 13 | Averia Serif Libre | 700 | 有机、旧印刷感 |

每款字体的版权和完整 OFL 1.1 许可证均内嵌在对比页内，也分别保存在 `fonts/` 中。新增五款的官方许可来源为 `https://raw.githubusercontent.com/google/fonts/main/ofl/{fredoka,baloo2,righteous,grandstander,pacifico}/OFL.txt`。

衬线方向新增字体的官方许可来源为 `https://raw.githubusercontent.com/google/fonts/main/ofl/{youngserif,caprasimo,breeserif,dmserifdisplay,averiaseriflibre}/OFL.txt`。Caprasimo 的来源说明见同目录下的 `caprasimo/DESCRIPTION.en_us.html`。

标记使用近黑色，以相向的折叠形态构成抽象 H；与 Hozon 字标横向组合。页面内保留透明背景，不加描边、投影或装饰容器。

## 独立标记生成记录

使用内置 image_gen 编辑工具，参考 `hozon-logo-concept-v1.png`，保留已认可的轮廓。最终提示词：

> Use case: background-extraction / logo-brand. Edit target: the approved Hozon horizontal logo in the attached image. Extract ONLY the existing symbol on the left as a production app icon, removing the Hozon wordmark and all empty horizontal space. Preserve the symbol's exact silhouette, two opposing rounded folded vertical forms, diagonal connecting/negative-space details, proportions and near-black solid color. No redesign, no new shapes, no lettering, no shadow, no gradient. Square canvas with genuine transparent background. Center the mark, occupying 88% of the square width, evenly padded, crisp flat edges. The approved symbol must remain recognizable and faithful.
