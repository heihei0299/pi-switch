package application

import "github.com/heihei0299/pi-switch/internal/protocol"

// ProviderPresets is the shared catalog for Native, HTTP and the existing CLI.
func ProviderPresets() []map[string]interface{} {
	all := []map[string]interface{}{
		{"id": "openai", "name": "OpenAI", "description": "OpenAI API", "websiteUrl": "https://openai.com", "api": protocol.OpenAIChat, "baseUrl": "https://api.openai.com/v1", "models": []string{"gpt-4o-mini", "gpt-4o", "o1"}},
		{"id": "anthropic", "name": "Anthropic", "description": "Anthropic API", "websiteUrl": "https://anthropic.com", "api": protocol.AnthropicMessages, "baseUrl": "https://api.anthropic.com", "models": []string{"claude-3-5-sonnet", "claude-3-opus"}},
		{"id": "google", "name": "Google", "description": "Google Gemini", "websiteUrl": "https://ai.google.dev", "api": protocol.GoogleGenerativeAI, "baseUrl": "https://generativelanguage.googleapis.com/v1", "models": []string{"gemini-pro"}},
		{"id": "deepseek", "name": "DeepSeek", "description": "DeepSeek", "websiteUrl": "https://deepseek.com", "api": protocol.OpenAIChat, "baseUrl": "https://api.deepseek.com/v1", "models": []string{"deepseek-chat"}},
	}
	// 预设必须与写入口能力一致：不可代理的 api 现在会被两个写入口拒绝，若仍在这里
	// 提供预设，用户点选后只会拿到一个必失败的保存。过滤依据是同一个能力来源
	// protocol.CanProxy，不维护第二份列表（见 system-contract §2.2）。
	usable := make([]map[string]interface{}, 0, len(all))
	for _, preset := range all {
		if api, _ := preset["api"].(string); protocol.CanProxy(api) {
			usable = append(usable, preset)
		}
	}
	return usable
}
