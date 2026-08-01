package prompt

import "strings"

const fakeModelIDPlaceholder = "{{FAKE_MODEL_ID}}"

// RenderPromptTemplate replaces the placeholder in the prompt asset with the real model name of the current request.
func RenderPromptTemplate(text string, modelName string) string {
	replacement := strings.TrimSpace(modelName)
	if replacement == "" {
		replacement = "current requested model"
	}
	return strings.ReplaceAll(text, fakeModelIDPlaceholder, replacement)
}
