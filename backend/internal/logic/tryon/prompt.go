// Package tryon 换装提示词模板，针对不同模型优化
package tryon

import "strings"

// BuildGPTSafety 构建 GPT-Image 专用的防封控安全提示词
func BuildGPTSafety(lang string) string {
	if lang == "zh" {
		return gptSafetyZhPrompt
	}
	return gptSafetyEnPrompt
}

// BuildGPT 构建 GPT-Image 的提示词（默认指向安全防封控模板）
func BuildGPT(lang string) string {
	return BuildGPTSafety(lang)
}

// BuildGemini 构建 Gemini（Nano Banana 2）图像模型的提示词（保留原提示词）
func BuildGemini(lang string) string {
	// Gemini 对中英文都支持良好，但换装任务英文指令更稳定
	return enPrompt
}

// BuildDefault 通用/其他模型（如豆包等）提示词（保留原提示词）
func BuildDefault(lang string) string {
	if lang == "zh" {
		return zhPrompt
	}
	return enPrompt
}

// gptSafetyEnPrompt GPT-Image-2 专用英文防封控安全提示词
const gptSafetyEnPrompt = `Task: Professional commercial fashion virtual try-on photograph.

Strict Requirements:
1. Model Identity: Maintain the adult female model from Image 1 - faithfully preserve her facial features, ethnic identity, eye shape, smile, skin tone, and hairstyle. Keep her identity consistent.
2. Wardrobe: Dress her in the exact clothing and accessories shown in Image 2. Faithfully reproduce the colors, fabric textures, patterns, and silhouette.
3. Safety & Fit: Professional high-end commercial lookbook standard. Fully clothed, modest neckline, standard tailored fit, upright standing posture, non-revealing.
4. Composition: Clean bright studio lighting, neutral minimal background, photorealistic 8k, high detail fashion editorial photograph.

Output: ONLY the final dressed-up photograph.`

// gptSafetyZhPrompt GPT-Image-2 专用中文防封控安全提示词
const gptSafetyZhPrompt = `任务：专业商业时尚虚拟试衣摄影。

严格要求：
1. 模特身份：保持第一张图片中的成年女性模特身份——忠实保留其面部特征、种族相貌、眼部轮廓、肤色与发型，保持人物相貌风格一致。
2. 服装穿搭：让人物完整穿上第二张图片中展示的服装与配饰，忠实还原服装的色彩、面料质感、图案与款式剪裁。
3. 安全与合规：采用高端商业型录摄影标准。穿着完整端庄，标准合身剪裁，端正站姿，避免过度露肤与暴露。
4. 画面效果：纯净明亮的商业影棚光照，简洁中性背景，逼真细节与质感，专业时尚画册品质。

输出：仅输出最终的完整着装照片。`

// enPrompt 英文提示词（主用，效果最稳定）
const enPrompt = `You are a professional fashion virtual try-on assistant.

Task: Dress the person in the first image wearing the garment in the second image, and output a single photorealistic full-body or half-body photo.

Strict requirements:
1. Keep the person's identity EXACTLY the same: same face, same facial features, same skin tone, same hairstyle, same body type and proportions. Do not change who they are.
2. Keep the person's original pose, body angle and orientation as much as possible; only adjust the arms/limbs if strictly needed to fit the garment naturally.
3. Put the exact garment from the second image onto the person: preserve its color, fabric texture, pattern, neckline, sleeve length, hem length and silhouette faithfully. Do not redesign or simplify the garment.
4. The garment must drape naturally on the body, follow the body contours, and cast realistic folds, shadows and highlights consistent with the original lighting.
5. Keep the original background, lighting and overall mood of the person's photo.
6. Photorealistic, high detail, professional fashion editorial quality, 8k, natural skin texture.

Output: ONLY the final dressed-up photograph, nothing else.`

// zhPrompt 中文提示词（备用）
const zhPrompt = `你是一名专业的时尚虚拟试衣助手。

任务：让第一张图片中的人物穿上第二张图片中的服装，输出一张逼真的全身或半身照片。

严格要求：
1. 人物身份必须完全一致：相同的脸、五官、肤色、发型、身材比例。不得改变人物是谁。
2. 尽量保持人物原有的姿势、身体角度和朝向；仅在必要时微调手臂/肢体以自然贴合服装。
3. 将第二张图中的服装原样穿到人物身上：忠实保留其颜色、面料质感、图案、领型、袖长、下摆长度和廓形。不得重新设计或简化服装。
4. 服装必须自然垂坠贴合身体曲线，并根据原始光照产生真实的褶皱、阴影和高光。
5. 保留人物照片原有的背景、光线和整体氛围。
6. 逼真、高细节、专业时尚大片质感、8k、自然皮肤纹理。

输出：仅输出最终的换装照片，不要其他内容。`

// AppendPoseInstruction 在基础提示词后追加姿势参考段：图 3 为姿势线画，
// 人物身份以图 1 为准、服装以图 2 为准、姿势与身体朝向以图 3 为准。
func AppendPoseInstruction(prompt, lang string) string {
	if lang == "zh" {
		return prompt + "\n\n姿势要求：第三张图片是姿势参考图（灰色人体模型线画）。让人物采用第三张图中人体的姿势、身体朝向与肢体位置；人物身份、相貌与发型仍以第一张图为准，服装仍以第二张图为准。不要把姿势参考图的灰色模型外观、线条风格或空白面部带入成图，成图保持真实照片质感。"
	}
	return prompt + "\n\nPose requirement: The third image is a pose reference (gray mannequin line drawing). Have the person adopt the body pose, orientation and limb positions of the figure in the third image; identity, face and hairstyle still come from the first image, and the garment still comes from the second image. Do not carry the gray mannequin look, line-art style or blank face of the pose reference into the result; keep the output a photorealistic photograph."
}

// AppendPersonNote 把用户填写的人物补充信息（身高、体型等）拼入提示词，帮助版型贴合。
func AppendPersonNote(prompt, note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return prompt
	}
	if r := []rune(note); len(r) > 200 {
		note = string(r[:200])
	}
	return prompt + "\n\n人物补充信息（供版型与比例参考）：" + note
}
