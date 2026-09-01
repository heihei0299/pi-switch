package scan

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type PiSession struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	LastActiveAt     *string `json:"lastActiveAt,omitempty"`
	Model            *string `json:"model,omitempty"`
	PromptTokensHint *uint64 `json:"promptTokensHint,omitempty"`
}

func SessionsDir() string {
	if p := os.Getenv("PI_CODING_AGENT_SESSION_DIR"); p != "" && strings.TrimSpace(p) != "" {
		return strings.TrimSpace(p)
	}
	if p := os.Getenv("PI_AGENT_SESSIONS"); p != "" && strings.TrimSpace(p) != "" {
		return strings.TrimSpace(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/pi-agent-sessions"
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

func Scan() map[string]PiSession {
	dir := SessionsDir()
	m := map[string]PiSession{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if info.Size() > 128*1024*1024 {
			return nil
		}
		sess := parseFile(path)
		if sess != nil {
			m[sess.ID] = *sess
		}
		return nil
	})
	return m
}

func parseFile(path string) *PiSession {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	if !scanner.Scan() {
		return nil
	}
	first := scanner.Text()
	var header map[string]interface{}
	if err := json.Unmarshal([]byte(first), &header); err != nil {
		return nil
	}
	if header["type"] != "session" {
		return nil
	}
	id, _ := header["id"].(string)
	if id == "" {
		return nil
	}
	cwd, _ := header["cwd"].(string)
	ts, _ := header["timestamp"].(string)
	title := cwd
	if idx := strings.LastIndex(cwd, "/"); idx >= 0 && idx+1 < len(cwd) {
		title = cwd[idx+1:]
	}
	if title == "" {
		title = id
	}
	var lastActive *string
	if ts != "" {
		cpy := ts
		lastActive = &cpy
	}
	var model *string
	var promptHint *uint64
	var firstUserText *string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var v map[string]interface{}
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			continue
		}
		typ, _ := v["type"].(string)
		if typ == "session_info" {
			if name, ok := v["name"].(string); ok && strings.TrimSpace(name) != "" {
				n := strings.TrimSpace(name)
				if len([]rune(n)) > 60 {
					n = string([]rune(n)[:60])
				}
				title = n
			}
		}
		if typ == "message" {
			if t, ok := v["timestamp"].(string); ok && t != "" {
				if lastActive == nil || t > *lastActive {
					cpy := t
					lastActive = &cpy
				}
			}
			if mid, ok := v["modelId"].(string); ok && mid != "" {
				cpy := mid
				model = &cpy
			} else if msg, ok := v["message"].(map[string]interface{}); ok {
				if mid, ok := msg["model"].(string); ok && mid != "" {
					cpy := mid
					model = &cpy
				}
				if firstUserText == nil {
					if role, _ := msg["role"].(string); role == "user" {
						if content, ok := msg["content"]; ok {
							var txt string
							if s, ok := content.(string); ok {
								txt = strings.TrimSpace(s)
							} else if arr, ok := content.([]interface{}); ok {
								for _, part := range arr {
									if pm, ok := part.(map[string]interface{}); ok && pm["type"] == "text" {
										if t, ok := pm["text"].(string); ok {
											if txt != "" {
												txt += " "
											}
											txt += t
										}
									}
								}
								txt = strings.TrimSpace(txt)
							}
							if txt != "" {
								if len([]rune(txt)) > 60 {
									txt = string([]rune(txt)[:60])
								}
								txt = strings.Map(func(r rune) rune {
									if r < 32 {
										return ' '
									}
									return r
								}, txt)
								txt = strings.TrimSpace(txt)
								cpy := txt
								firstUserText = &cpy
							}
						}
					}
				}
				if usage, ok := msg["usage"].(map[string]interface{}); ok {
					if inp, ok := usage["input"].(float64); ok {
						u := uint64(inp)
						promptHint = &u
					} else if inp, ok := usage["prompt_tokens"].(float64); ok {
						u := uint64(inp)
						promptHint = &u
					}
				}
			}
		}
		if typ == "model_change" {
			if mid, ok := v["modelId"].(string); ok && mid != "" {
				cpy := mid
				model = &cpy
			}
		}
	}
	if firstUserText != nil {
		basename := cwd
		if idx := strings.LastIndex(cwd, "/"); idx >= 0 {
			basename = cwd[idx+1:]
		}
		if title == basename || title == "" {
			title = *firstUserText
		}
	}
	return &PiSession{ID: id, Title: title, LastActiveAt: lastActive, Model: model, PromptTokensHint: promptHint}
}
