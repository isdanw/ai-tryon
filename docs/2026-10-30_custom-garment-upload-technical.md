# 自定义上传服装技术实施文档

> 文档日期：2026-10-30
> 适用项目：`nvzhuang`
> 功能目标：在 AI 试穿中允许用户上传自己的服装图片，并与现有图库服装使用同一套生成接口
> 核心约束：不新增后端上传接口，不持久化用户服装原图，不影响现有图库选衣流程

> **实现更新（优先于下文早期单图示例）**：自定义服装现支持最多 2 张图片。上传 1 张时按“完整全身服装”处理；上传 2 张时，第 1 张按上半身服装、第 2 张按下半身服装处理，前端合成为一张纵向参考图后继续写入现有 `garment_image` 字段。人物图或单张服装图任一边小于 256 px 时，生成前自动等比放大到合规尺寸。AI 换装校验或接口失败时，同时更新页内提示并弹出可聚焦错误弹窗。页面默认使用白色主题，并支持持久化切换黑色 / 白色主题。

## 1. 功能概述

当前 AI 试穿流程只能从项目内置图库选择服装。生成请求已经通过 `garment_image` 接收服装图片 Base64，后端也已经对人像和服装图执行统一格式、大小、尺寸及像素校验，因此新增“自定义上传服装”主要是前端状态、交互和图片处理能力扩展。

本功能完成后，用户可以在“选择一件服装”步骤中切换两种来源：

1. 图库服装：保留现有搜索、随机和服装卡片选择。
2. 上传服装：选择本地 JPEG、PNG 或 WebP 图片，预览后直接用于 AI 换装。

两种来源最终都转换为 `garment_image` Base64，并继续调用现有 `POST /tryon`，后端 API 请求结构保持兼容。

## 2. 范围与非目标

本需求包含：

- 在试穿步骤 2 增加“图库服装”和“上传服装”来源切换。
- 上传、预览、替换和移除自定义服装。
- 对上传服装执行格式、文件大小、宽高和像素校验。
- 复用现有 `prepareTryonImage` 压缩和 Base64 转换逻辑。
- 统一图库服装和上传服装的选中状态。
- 更新当前选中服装提示条。
- 允许使用上传服装进行淘宝和拼多多以图搜款。
- 在生成历史中标记“图库服装”或“自定义上传”。
- 增加中英文文案、键盘操作和基础可访问性。
- 增加相应前端测试或可独立执行的人工验收用例。

本需求不包含：

- 把用户服装上传到服务器长期保存。
- 新增服装素材库、用户账号或跨设备同步。
- 修改后端 `POST /tryon` 请求结构。
- 新增数据库表或对象存储。
- 自动识别服装类别、颜色、品牌或商品信息。
- 自动抠图、去背景或人体分割。
- 一次上传多件服装或多服装叠穿。
- 将用户上传的服装加入公共图库。

## 3. 当前代码依据

### 3.1 Evidence

| ID | 代码位置 | 当前观察 | 复核命令 |
|---|---|---|---|
| E-001 | `index.html` 的步骤 2 | 当前只有搜索、随机和 `fitGarmentList` 图库选择 | `rg -n "fitGarmentTitle|fitGarmentList|fitRandom" index.html` |
| E-002 | `index.html` 的试穿状态 | 当前使用 `pickedGarment` 保存图库数据对象 | `rg -n "let pickedGarment|selectGarment" index.html` |
| E-003 | `index.html` 的 `doTryon` | 服装图固定从 `assets/` 加载并转换为 Base64 | `rg -n "garmentB64|pickedGarment.画像" index.html` |
| E-004 | `backend/api/tryon/v1/tryon.go` | 后端已经通过 `garment_image` 接收任意服装 Base64 | `rg -n "GarmentImage|garment_image" backend/api/tryon/v1/tryon.go` |
| E-005 | `backend/internal/imagevalidate` | 后端对人像和服装图使用同一个验证器 | `rg -n "ValidateImagesPair|ValidateImage" backend/internal/imagevalidate` |

### 3.2 Findings

| ID | 结论 | 依据 | 实施影响 |
|---|---|---|---|
| F-001 | 后端不需要区分服装来自图库还是本地上传 | E-004、E-005 | 不新增接口，只改前端图片来源 |
| F-002 | `pickedGarment` 与图库字段强绑定 | E-001、E-002、E-003 | 需要引入统一的选中服装模型 |
| F-003 | 现有图片压缩和错误码可以直接复用 | E-003、E-005 | 避免新增第二套图片处理逻辑 |

### 3.3 目标调用路径

1. 用户选择“图库服装”或“上传服装”。
2. 前端得到统一的 `selectedGarment`。
3. 前端取出 `selectedGarment.imageSource`。
4. `prepareTryonImage` 校验、缩放、压缩并返回 Base64。
5. Base64 写入现有 `garment_image`。
6. 后端继续使用 `ValidateImagesPair` 验证两张图片。
7. 现有模型适配器完成生成。

## 4. 产品交互要求

### 4.1 服装来源切换

在步骤 2 标题下方增加两个切换按钮，语义上使用 Tab：

- 图库服装
- 上传服装

建议结构：

```html
<div class="garment-source-tabs" role="tablist" aria-label="服装来源">
  <button type="button" id="garmentCatalogTab" role="tab" aria-selected="true" aria-controls="garmentCatalogPanel">图库服装</button>
  <button type="button" id="garmentUploadTab" role="tab" aria-selected="false" aria-controls="garmentUploadPanel">上传服装</button>
</div>
```

交互规则：

- 默认进入“图库服装”。
- 切换来源不会删除另一个来源已经选择或上传的内容。
- 当前 Tab 决定生成时使用哪一件服装。
- 切换到图库后，使用最近一次选中的图库服装；没有选中过则视为未选服装。
- 切换到上传后，使用最近一次上传的服装；没有上传则视为未选服装。
- 页面刷新后恢复为图库模式，自定义上传内容不恢复。
- Tab 支持左右方向键切换、Enter 和空格激活。

### 4.2 图库服装面板

图库面板保留现有功能：

- 搜索框。
- 随机按钮。
- 服装网格。
- 当前选中高亮。

在上传模式下隐藏图库面板，但不要清空搜索词或 `pickedGarment`。

### 4.3 上传服装面板

上传面板包含：

- 上传区域。
- 支持格式和大小提示。
- 上传后的图片预览。
- 文件名或“自定义上传”标签。
- 替换图片按钮。
- 移除图片按钮。
- 拍摄建议提示。

建议结构：

```html
<div id="garmentUploadPanel" role="tabpanel" aria-labelledby="garmentUploadTab" hidden>
  <label class="upload-zone garment-upload-zone" for="fitGarmentFile">
    <span class="ico" aria-hidden="true">👗</span>
    <span data-i18n="fitGarmentUploadBtn">点击上传服装图片</span>
    <small data-i18n="fitGarmentUploadHint">支持 JPG / PNG / WebP，建议完整展示单件服装</small>
  </label>
  <input type="file" id="fitGarmentFile" accept="image/jpeg,image/png,image/webp" hidden>
  <div class="custom-garment-preview" id="customGarmentPreview" hidden>
    <img id="customGarmentImage" alt="">
    <div class="custom-garment-meta">
      <strong id="customGarmentName"></strong>
      <span id="customGarmentInfo"></span>
    </div>
    <button type="button" id="customGarmentReplace">替换</button>
    <button type="button" id="customGarmentRemove">移除</button>
  </div>
</div>
```

### 4.4 上传指导文案

上传区域需要提示：

- 建议只出现一件主要服装。
- 建议正面、完整、清晰展示。
- 建议使用纯色或简单背景。
- 避免严重遮挡、折叠、低清晰度和多件衣物重叠。
- 可以上传平铺图、衣架图、模特图或人台图，但实际效果由模型决定。

这些是效果建议，不作为强制校验规则。

### 4.5 当前选中服装提示条

现有 `#aiPicked` 同时支持两种来源。

图库服装显示：

- 图库缩略图。
- `已选服装`。
- 服装 ID。
- 淘宝搜同款。
- 拼多多搜同款。

上传服装显示：

- 上传图片预览。
- `自定义上传`。
- 清理后的文件名，最长显示 80 个字符。
- 淘宝搜同款。
- 拼多多搜同款。

未选服装显示：

- 图库模式：提示用户从图库选择。
- 上传模式：提示用户上传服装图片。

## 5. 前端状态设计

### 5.1 不继续扩展 `pickedGarment`

`pickedGarment` 是 `data.js` 的图库数据对象，依赖 `id`、`サムネ`、`画像` 等字段。不要给上传文件伪造这些字段，否则后续搜索、随机和同款功能容易出现分支错误。

保留 `pickedGarment` 作为图库内部状态，同时新增：

```javascript
let garmentMode = 'catalog';
let pickedGarment = null;
let uploadedGarment = null;
```

`uploadedGarment` 结构：

```javascript
{
  source: 'upload',
  fileName: 'dress.webp',
  mimeType: 'image/webp',
  fileSize: 1234567,
  width: 1200,
  height: 1800,
  objectUrl: 'blob:...',
  uploadedAt: 1793318400000
}
```

### 5.2 统一选中服装模型

新增纯函数 `getSelectedGarment()`，向生成、预览、历史和同款搜索提供统一结构：

```javascript
function getSelectedGarment(){
  if(garmentMode === 'upload'){
    if(!uploadedGarment) return null;
    return {
      source: 'upload',
      id: '',
      label: uploadedGarment.fileName || t('customGarmentLabel'),
      imageSource: uploadedGarment.objectUrl,
      previewSource: uploadedGarment.objectUrl,
    };
  }
  if(!pickedGarment) return null;
  return {
    source: 'catalog',
    id: pickedGarment.id,
    label: pickedGarment.id,
    imageSource: 'assets/' + pickedGarment.画像,
    previewSource: 'assets/' + pickedGarment.サムネ,
  };
}
```

要求：

- `doTryon`、`refreshPicked`、同款搜索和历史保存都调用该函数。
- 业务代码不得继续直接假设 `pickedGarment` 一定存在。
- `window.openFitWith(garmentId)` 必须自动切换到 `catalog` 模式。
- 随机选择和点击图库卡片必须自动切换到 `catalog` 模式。
- 上传成功必须自动切换到 `upload` 模式。

## 6. 上传和图片处理

### 6.1 文件选择

`#fitGarmentFile` 只接受：

- JPEG
- PNG
- WebP

前端初步限制：

| 项目 | 限制 |
|---|---:|
| 原始文件大小 | 最大 12 MiB |
| 原图最小宽度 | 不拦截；生成前自动等比放大到至少 256 px |
| 原图最小高度 | 不拦截；生成前自动等比放大到至少 256 px |
| 最大宽度 | 8192 px |
| 最大高度 | 8192 px |
| 最大总像素 | 20,000,000 px |

选择文件后的处理顺序：

1. 检查 `File.type` 是否属于允许类型。
2. 检查原始文件大小。
3. 创建临时 Object URL。
4. 用 `Image` 解码并读取真实宽高。
5. 检查宽高和总像素。
6. 解码成功后写入 `uploadedGarment`。
7. 自动切换到上传模式。
8. 刷新上传预览和当前选中服装提示条。

浏览器端检查仅用于快速反馈；后端继续使用 `ValidateImagesPair` 做最终校验。

### 6.2 复用现有压缩函数

生成时不直接读取文件为 Base64，统一调用现有：

```javascript
prepareTryonImage(selectedGarment.imageSource, shouldCompress)
```

这样上传服装与图库服装使用相同规则：

- 开启压缩时最长边 1280 像素。
- 高清模式最长边 2048 像素。
- 任一边小于 256 像素时等比放大，确保发送给后端的宽、高均不少于 256 像素。
- 优先编码为 WebP。
- 浏览器不支持 WebP 时编码为 JPEG。
- 逐级调整质量直到不超过 8 MiB。
- 后端再次校验格式、大小、尺寸和像素。

不新增 `FileReader.readAsDataURL` 的独立压缩流程，避免出现两套质量和限制不一致的逻辑。

### 6.3 Object URL 生命周期

自定义上传图片使用 `URL.createObjectURL(file)` 预览。

要求：

- 替换图片前调用 `URL.revokeObjectURL` 释放旧 URL。
- 移除图片时释放 URL。
- 页面 `beforeunload` 时释放仍存在的 URL。
- 试穿弹窗关闭时不自动释放，确保用户再次打开时不必重新上传。
- 生成请求尚未完成时不能释放当前使用的 URL。
- 图片加载失败时立即释放临时 URL。

### 6.4 文件名处理

文件名只用于浏览器界面和本地历史标签，不发送给后端。

要求：

- 使用浏览器提供的 `File.name`，不读取本地完整路径。
- 移除控制字符。
- 显示长度限制为 80 个字符。
- 写入 HTML 时必须经过现有 `esc()`。
- 下载文件名不能直接拼接未经清理的上传文件名。

## 7. 生成请求改造

当前逻辑：

```javascript
garmentB64 = await prepareTryonImage('assets/' + pickedGarment.画像, shouldCompress);
```

改为：

```javascript
const selectedGarment = getSelectedGarment();
if(!selectedGarment){
  showHint(garmentMode === 'upload' ? t('aiNeedUploadedGarment') : t('aiNeedGarment'));
  return;
}

garmentB64 = await prepareTryonImage(selectedGarment.imageSource, shouldCompress);
```

请求体保持不变：

```json
{
  "person_image": "base64...",
  "garment_image": "base64...",
  "model": "gemini",
  "size": "1024x1536",
  "lang": "zh"
}
```

禁止新增以下字段作为功能依赖：

- `garment_file_name`
- `garment_source`
- `garment_url`

后端只需要服装图片内容，来源信息仅在前端本地使用。

## 8. 同款搜索改造

当前选中服装提示条的淘宝和拼多多按钮必须使用统一来源：

```javascript
const selected = getSelectedGarment();
if(!selected) return;
searchTaobaoByImage(selected.imageSource, buildGarmentSearchFilename(selected), button);
```

文件名规则：

- 图库：`taobao-AUT-0001.png` 或 `pinduoduo-AUT-0001.png`。
- 上传：`taobao-custom-garment.png` 或 `pinduoduo-custom-garment.png`。

要求：

- 自定义服装可以使用现有剪贴板复制、PNG 转换和下载引导。
- 搜同款过程中不得修改或释放上传图片的 Object URL。
- AI 生成结果区域的搜同款逻辑不需要修改。

## 9. 生成历史改造

当前 IndexedDB 历史记录只保存结果图，不保存用户输入原图。继续保持这一隐私策略。

历史记录增加可选字段：

```javascript
{
  garmentSource: 'catalog' | 'upload',
  garmentId: 'AUT-0001',
  garmentLabel: 'AUT-0001' | '自定义上传'
}
```

保存规则：

| 来源 | `garmentSource` | `garmentId` | `garmentLabel` |
|---|---|---|---|
| 图库 | `catalog` | 图库 ID | 图库 ID |
| 上传 | `upload` | 空字符串 | 本地化的“自定义上传” |

要求：

- 不保存自定义服装原图 Blob。
- 不保存用户原始文件名，避免历史记录泄漏本地命名信息。
- 不需要升级 IndexedDB 版本；现有对象存储允许增加字段。
- 旧历史记录缺少 `garmentSource` 时，按 `garmentId` 是否存在兼容显示。

## 10. 隐私和安全要求

- 上传的服装原图只保存在当前页面内存和浏览器 Blob URL 中。
- 不写入 localStorage、sessionStorage 或 IndexedDB。
- 不发送文件名、完整路径、EXIF 文本或额外文件元数据到后端。
- Canvas 重编码后再发送，可以自然移除大部分 EXIF 元数据。
- 后端继续执行真实 MIME、文件大小、宽高和像素校验。
- 前端不得把 Object URL、文件名或图片 Base64 写入控制台。
- 错误信息不得包含本地文件路径。
- 页面刷新或关闭后，自定义上传内容不可恢复。

## 11. 国际化文案

至少增加以下 i18n Key：

| Key | 中文 | English |
|---|---|---|
| `garmentSourceCatalog` | 图库服装 | Gallery garments |
| `garmentSourceUpload` | 上传服装 | Upload garment |
| `fitGarmentUploadBtn` | 点击上传服装图片 | Click to upload a garment |
| `fitGarmentUploadHint` | 支持 JPG / PNG / WebP，建议完整展示单件服装 | JPG / PNG / WebP; show one complete garment |
| `customGarmentLabel` | 自定义上传 | Custom upload |
| `customGarmentReplace` | 替换图片 | Replace image |
| `customGarmentRemove` | 移除图片 | Remove image |
| `customGarmentGuide` | 建议正面、完整、清晰展示单件服装 | Use a clear front view of one complete garment |
| `aiNeedUploadedGarment` | 请先上传服装图片 | Upload a garment image first |
| `customGarmentReady` | 自定义服装已就绪 | Custom garment is ready |

语言切换时必须刷新：

- Tab 文案。
- 上传区域。
- 按钮文案。
- 当前选中服装提示条。
- 生成历史中的“自定义上传”标签。

## 12. 可访问性要求

- 来源切换使用 `role="tablist"`、`role="tab"` 和 `role="tabpanel"`。
- 当前 Tab 正确设置 `aria-selected` 和 `tabindex`。
- 上传预览图片的 `alt` 使用“自定义上传服装预览”或英文等价文本。
- 替换和移除按钮有清晰的可访问名称。
- 键盘用户可以完成来源切换、上传、替换、移除和生成。
- 移除服装后，焦点返回上传按钮。
- 上传失败后通过现有提示区域告知原因，不只依赖颜色变化。
- 当前选中状态继续通过 `aria-pressed` 表达。

## 13. 样式要求

新增样式必须复用现有深色主题变量。

建议新增类：

```text
.garment-source-tabs
.garment-source-tab
.garment-source-tab.active
.garment-source-panel
.garment-upload-zone
.custom-garment-preview
.custom-garment-preview img
.custom-garment-meta
.custom-garment-actions
```

响应式要求：

- 桌面端上传预览可以使用左右布局。
- 560px 以下改为上下布局。
- 预览图保持合理比例，使用 `object-fit: contain`，不能强制裁切服装主体。
- 预览容器最大高度建议为 360px。
- 替换和移除按钮在移动端保持至少 44px 点击高度。
- 上传模式隐藏图库搜索和随机按钮，避免用户误以为它们会作用于上传图片。

## 14. 错误处理

前端复用已有错误文案和业务码：

| 场景 | 前端错误码或服务端 `error_code` | 行为 |
|---|---|---|
| 文件格式不支持 | `UNSUPPORTED_IMAGE_TYPE` | 清空文件输入并提示支持格式 |
| 图片无法解码 | `INVALID_IMAGE_BASE64` | 释放临时 URL并提示重新选择 |
| 文件过大 | `IMAGE_FILE_TOO_LARGE` | 提示压缩或更换图片 |
| 宽高不合法 | `IMAGE_DIMENSIONS_INVALID` | 提示尺寸要求 |
| 总像素过高 | `IMAGE_PIXEL_LIMIT_EXCEEDED` | 提示缩小图片 |
| 未上传服装 | `aiNeedUploadedGarment` | 不发请求，焦点移到上传区域 |

要求：

- 选择失败后将 `fitGarmentFile.value` 清空，允许再次选择同一个文件。
- 替换失败时保留上一张有效上传服装，不要直接清空当前状态。
- 后端校验失败时保留当前上传内容，方便用户调整压缩选项后重试。
- 生成失败不清空已上传服装。

## 15. 代码改造范围

主要修改：

| 文件 | 改造内容 |
|---|---|
| `index.html` | 新增来源 Tab、上传面板、状态模型、生成来源选择、历史字段、i18n 和样式 |
| `README.md` | 功能列表增加自定义上传服装和隐私说明 |
| `docs/2026-10-30_custom-garment-upload-technical.md` | 保留为实现与验收依据 |

后端通常无需修改：

| 文件 | 原因 |
|---|---|
| `backend/api/tryon/v1/tryon.go` | 已支持 `garment_image` Base64 |
| `backend/internal/controller/tryon/tryon_v1_tryon.go` | 已统一校验人像和服装图 |
| `backend/internal/imagevalidate/*` | 已支持 JPEG、PNG、WebP 和尺寸限制 |

只有当实现过程中发现图库 WebP 与上传 WebP 的验证行为不一致时，才允许修复后端验证缺陷；不能为区分上传来源新增接口字段。

## 16. 实施步骤

建议其他 AI 按以下顺序修改：

1. 增加步骤 2 的来源 Tab 和上传面板 HTML。
2. 增加中英文 i18n 文案。
3. 增加上传面板和移动端 CSS。
4. 引入 `garmentMode` 和 `uploadedGarment` 状态。
5. 实现上传、预览、替换、移除和 Object URL 清理。
6. 实现 `getSelectedGarment()`。
7. 改造 `doTryon()` 使用统一服装来源。
8. 改造 `refreshPicked()` 和同款搜索。
9. 改造生成历史的来源和标签。
10. 更新 README。
11. 完成测试和人工验收。

建议提交信息：

```text
feat: support custom garment uploads in AI try-on
```

## 17. 测试要求

### 17.1 上传基础流程

- 上传合法 JPEG 后显示预览并自动切换到上传模式。
- 上传合法 PNG 和 WebP 均能生成。
- 替换图片后预览和生成请求使用新图片。
- 移除图片后上传模式变为未选状态。
- 重新选择相同文件可以再次触发处理。

### 17.2 来源切换

- 先选图库服装，再上传服装，生成使用上传服装。
- 上传服装后切换图库，生成使用最近选中的图库服装。
- 再切回上传模式，原上传服装仍然存在并被使用。
- `openFitWith(id)` 自动切换到图库模式并选择指定服装。
- 随机按钮只改变图库选择，不删除上传服装。

### 17.3 请求验证

Mock `fetch` 并检查：

- `garment_image` 来自当前活动来源。
- 自定义上传时请求中不包含文件名或 Object URL。
- 请求结构仍与旧接口兼容。
- 后端 Token 和自定义模型配置行为不受影响。

### 17.4 异常场景

- 非图片文件被拒绝。
- 损坏图片被拒绝并释放临时 URL。
- 超过文件大小限制被拒绝。
- 尺寸过小的有效图片会在生成前自动等比放大，不再直接拒绝。
- 尺寸超过 8192 像素或像素总量超限的图片被拒绝。
- 替换失败时上一张有效服装仍然可用。
- 后端返回图片错误码时上传内容不会被清空。

### 17.5 历史和隐私

- 图库结果历史显示服装 ID。
- 上传结果历史显示“自定义上传”。
- IndexedDB 中没有保存上传服装 Blob 和原始文件名。
- localStorage 和 sessionStorage 中没有上传图片数据。
- 页面刷新后上传服装消失。

### 17.6 可访问性

- 键盘可以切换来源 Tab。
- 上传、替换、移除和生成都可通过键盘完成。
- Tab 的 `aria-selected` 与实际面板一致。
- 隐藏面板不会继续进入 Tab 键顺序。
- 中英文切换后所有新增文案正确刷新。

## 18. 验收标准

- [ ] 用户可以在图库服装和上传服装之间切换。
- [ ] 用户可以上传、预览、替换和移除 JPEG、PNG、WebP 服装图。
- [ ] 上传服装经过现有压缩流程后写入 `garment_image`。
- [ ] 后端接口和请求结构没有新增必填字段。
- [ ] 图库选择、搜索、随机和 `openFitWith` 行为保持正常。
- [ ] 当前选中提示条能正确显示两种服装来源。
- [ ] 上传服装可以用于淘宝和拼多多以图搜款。
- [ ] 历史记录只保存结果图和“自定义上传”标签，不保存上传服装原图。
- [ ] 上传图片不会写入 localStorage、sessionStorage 或 IndexedDB。
- [ ] 替换和移除操作正确释放 Object URL。
- [ ] 中英文和键盘操作可用。
- [ ] 现有后端图片验证测试与前端试穿流程没有回归。

## 19. 交付要求

实现 AI 完成后必须提供：

1. 修改后的 `index.html`。
2. 更新后的 `README.md`。
3. 新增或更新的测试结果。
4. 浏览器手工验证结果，至少覆盖 Chrome 桌面端和移动端窄屏布局。
5. 变更文件清单。
6. 对 Object URL、IndexedDB 和请求载荷的隐私检查说明。

不得在本功能中顺带新增服务器文件存储、用户账号、异步任务或公共服装上传图库。
