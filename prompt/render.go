package prompt

import "strings"

const fakeModelIDPlaceholder = "{{FAKE_MODEL_ID}}"

// RenderPromptTemplate replaces the placeholder in the prompt asset with the real model name of the current request.
func RenderPromptTemplate(text string, modelName string) string {
	replacement := strings.TrimSpace(modelName)
	if replacement == "" {
		replacement = "当前请求模型"
	}
	return strings.ReplaceAll(text, fakeModelIDPlaceholder, replacement)
}
