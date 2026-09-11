package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/protocol"
)

// 从 payload 判一条 profile 的三个写入口——整文件门（PUT /api/config）、CRUD 门
// （POST /api/profiles）、advisory（GET /api/config/validate）——必须在同一形状上给出可预期的
// 口径。这张「形状 × api」表就是那个不变量：以后任何一次「某个门在某形状上口径变了」都要在下面
// 某一行上以 diff 的形式出现，而不是等下一轮人工审查发现（flat profile 绕过 capability、
// 「无 channel 又无连接信息」的形状漏判，都是这么被发现的）。
//
// 表里只断言判决（放行/拒绝）和 advisory 是否报出 capability 类 error；消息措辞由
// TestPutConfig_CapabilityMatrix / TestProfileCRUD_CapabilityMatrix 钉住。duplicate 是第四个
// 写入口，但它复制磁盘上既有的 profile、不从 payload 构造形状，所以单独测（见本文件末尾）。
func TestDoorParity_CapabilityVerdicts(t *testing.T) {
	const (
		shapeFlatNoURL     = "flat-no-url"
		shapeFlatWithURL   = "flat-with-url"
		shapeChannel       = "channel"
		shapeChannelOwnAPI = "channel-own-api"
		shapeEmpty         = "empty"
		// emptyConfig 是每个门自己的起点：拒绝过的 payload 不该留下状态影响下一格。
		emptyConfig = `{"version":2,"profiles":{}}`
	)

	cases := []struct {
		shape    string
		api      string
		configOK bool // PUT /api/config
		crudOK   bool // POST /api/profiles
		advisory bool // advisory 是否报出 capability 类 error
		note     string
	}{
		{shapeFlatNoURL, protocol.OpenAIChat, true, true, false, "能力上可代理：整文件门不跑 shape 校验；缺 baseUrl 只有 advisory 报 error，CRUD 门不要求 flat profile 带 baseUrl"},
		{shapeFlatNoURL, protocol.GoogleGenerativeAI, false, false, true, "声明了 api 就判，没有 channel 也不例外"},
		{shapeFlatNoURL, "unknown-api", false, false, true, "同上，未知 api"},
		{shapeFlatWithURL, protocol.OpenAIChat, true, true, false, "legacy flat profile：ResolvedUpstreams 用与 runtime 相同的 effective upstream fallback 语义；这里只验证 capability 判定，不声明 synthesized channel 一定能走完 route resolution"},
		{shapeFlatWithURL, protocol.GoogleGenerativeAI, false, false, true, "capability matrix 覆盖的经典形状"},
		{shapeFlatWithURL, "unknown-api", false, false, true, ""},
		{shapeFlatWithURL, "", false, false, true, "连接信息在、api 不在：合成 channel 的 effective api 为空"},
		{shapeChannel, protocol.OpenAIChat, true, true, false, ""},
		{shapeChannel, protocol.GoogleGenerativeAI, false, false, true, "逐 channel 判 effective api"},
		{shapeChannel, "unknown-api", false, false, true, ""},
		{shapeChannel, "", false, false, true, "channel 与 profile 都没有 api"},
		{shapeChannelOwnAPI, protocol.OpenAIChat, true, true, true, "profile 顶层没有 api 而 channel 自带：两道理性写门按 channel 的 effective api 放行，advisory 仍按 profile 内容报 api required——第二处已记录的分歧"},
		{shapeChannelOwnAPI, protocol.GoogleGenerativeAI, false, false, true, "同上，但 channel 自己的能力就不可代理：写门也拒"},
		{shapeChannelOwnAPI, "unknown-api", false, false, true, ""},
		{shapeFlatNoURL, "", true, false, true, "没有任何可判的 api：整文件门（与 duplicate）放行，CRUD 门与 advisory 拒收/报错"},
		{shapeEmpty, "", true, false, true, "逐字空的 profile，判定与上一行相同"},
	}

	for _, tc := range cases {
		t.Run(tc.shape+"/"+apiLabel(tc.api), func(t *testing.T) {
			prof := doorParityProfile(tc.shape, tc.api)
			body := `{"version":2,"profiles":{"p":` + prof + `}}`
			r := NewMgmtRouter()

			check := func(door string, wantOK bool, code int, respBody string) {
				t.Helper()
				if wantOK && code != http.StatusOK {
					t.Fatalf("%s：形状 %s、api %q 期望放行，得到 %d（%s）%s", door, tc.shape, tc.api, code, strings.TrimSpace(respBody), tc.note)
				}
				// 拒绝只有一种码：能力/shape 规则给 400。出现 500 说明是存储问题，不能算「按预期拒绝」。
				if !wantOK && code != http.StatusBadRequest {
					t.Fatalf("%s：形状 %s、api %q 期望 400，得到 %d（%s）%s", door, tc.shape, tc.api, code, strings.TrimSpace(respBody), tc.note)
				}
			}

			writeChannelConfig(t, t.TempDir(), emptyConfig)
			req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			check("PUT /api/config", tc.configOK, w.Code, w.Body.String())

			writeChannelConfig(t, t.TempDir(), emptyConfig)
			posted := callMgmt(r, http.MethodPost, "/api/profiles", `{"name":"p","profile":`+prof+`}`)
			check("POST /api/profiles", tc.crudOK, posted.Code, posted.Body.String())

			// advisory 读磁盘上的同一条 profile，所以先把它原样写进去（不经过任何门）。
			writeChannelConfig(t, t.TempDir(), body)
			adv := callMgmt(r, http.MethodGet, "/api/config/validate", "")
			if adv.Code != http.StatusOK {
				t.Fatalf("GET /api/config/validate = %d (%s)", adv.Code, adv.Body.String())
			}
			var issues []map[string]interface{}
			if err := json.Unmarshal(adv.Body.Bytes(), &issues); err != nil {
				t.Fatalf("advisory 响应不是 issue 列表: %v (%s)", err, adv.Body.String())
			}
			got := false
			for _, issue := range issues {
				level, _ := issue["level"].(string)
				message, _ := issue["message"].(string)
				if level == "error" && isCapabilityMessage(message) {
					got = true
				}
			}
			if got != tc.advisory {
				t.Fatalf("advisory capability error = %v, want %v（形状 %s、api %q）issues=%v", got, tc.advisory, tc.shape, tc.api, issues)
			}
		})
	}
}

// doorParityProfile 构造表里的形状。responsesMode 一律不声明（等于 auto），这样表里只有
// capability 一条规则在起作用。
func doorParityProfile(shape, api string) string {
	switch shape {
	case "flat-no-url":
		return `{"api":"` + api + `"}`
	case "flat-with-url":
		return `{"api":"` + api + `","baseUrl":"https://example.test/v1","apiKey":"k"}`
	case "channel":
		return `{"api":"` + api + `","baseUrl":"https://example.test/v1","apiKey":"k",` +
			`"upstreams":[{"name":"main","api":"` + api + `","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}`
	case "channel-own-api":
		// profile 顶层不声明 api，channel 自带——运行期按 channel 的 api 走。
		return `{"baseUrl":"https://example.test/v1","apiKey":"k",` +
			`"upstreams":[{"name":"main","api":"` + api + `","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}`
	default: // empty：完全空白的 profile
		return `{}`
	}
}

// isCapabilityMessage 只认 advisory 对 api/mode 组合本身的判定：缺失、未知、已知但当前不可代理。
// 这些串是 advisory 对使用者的话术，属于它的对外契约，所以这里按原话断言；"api required" 用精确
// 相等而不是前缀匹配，channel 级规则的 "api is required for each upstream" 因此不会被误算进来。
// 其他 error（缺 baseUrl、channel 名非法等）属于 shape 诊断，不在这张表的口径里。
func isCapabilityMessage(message string) bool {
	return message == "api required" ||
		strings.HasPrefix(message, "unsupported api ") ||
		strings.Contains(message, "is not currently proxy-supported")
}

func apiLabel(api string) string {
	if api == "" {
		return "no-api"
	}
	return api
}

// duplicate 是第四个写入口，但它不从 payload 构造 profile，而是复制磁盘上既有的一条，所以它的
// 期望按「源 profile 的形状」声明。规则是：**副本不能是整文件门会拒绝的配置**——能力判定与那道
// 门共用 `config.ValidateResolvedCapability`（逐 channel + flat），而 shape/模型/channel 名/retry
// 一律不判，否则磁盘上带这些问题的 profile 连「复制一份再改」这条修复路径都会断掉。这些源只可能
// 来自遗留配置或手工编辑（三道授权门已经拒收这些形状），所以用写盘来构造。
func TestDoorParity_DuplicateSourceVerdicts(t *testing.T) {
	channel := func(channelAPI, channelBaseURL string) string {
		return `{"api":"` + channelAPI + `","baseUrl":"https://example.test/v1","apiKey":"k",` +
			`"upstreams":[{"name":"main","api":"` + channelAPI + `","baseUrl":"` + channelBaseURL + `","apiKey":"k","models":[{"id":"m1"}]}]}`
	}

	cases := []struct {
		name     string
		source   string
		wantCode int
		wantMsg  string // 非空时断言拒绝文案，证明点名的是哪条规则/哪个 channel
		note     string
	}{
		{"flat unproxyable", `{"api":"` + protocol.GoogleGenerativeAI + `","baseUrl":"https://example.test/v1","apiKey":"k"}`, 400, "not currently proxy-supported", "再复制一份跑不起来的 legacy profile 等于用 CRUD 造一个新的必失败配置"},
		{"flat unknown api", `{"api":"unknown-api","baseUrl":"https://example.test/v1","apiKey":"k"}`, 400, "unsupported api unknown-api", ""},
		{"flat api-less with connection info", `{"baseUrl":"https://example.test/v1","apiKey":"k"}`, 400, "api is required", "连接信息在、api 不在：整文件门也会拒（合成 channel 的 effective api 为空）"},
		{"flat no content at all", `{}`, 200, "", "完全空白的源：没有任何可判的组合，与整文件门同样放行"},
		{"flat proxyable", `{"api":"` + protocol.OpenAIChat + `","baseUrl":"https://example.test/v1","apiKey":"k"}`, 200, "", "可代理的 legacy profile 照样能复制"},
		{"flat shape problem", `{"api":"` + protocol.OpenAIChat + `","baseUrl":"ftp://example.test","apiKey":"k"}`, 200, "", "shape 不归 duplicate 管：坏 baseUrl 的源仍可复制一份再改"},
		{"channel unproxyable", channel(protocol.GoogleGenerativeAI, "https://example.test/v1"), 400, "upstreams[0]: api " + protocol.GoogleGenerativeAI, "channel 型源的能力判定与整文件门同一条（不再是只判 flat）"},
		{"channel unknown api", channel("unknown-api", "https://example.test/v1"), 400, "unsupported api unknown-api", ""},
		{"channel proxyable", channel(protocol.OpenAIChat, "https://example.test/v1"), 200, "", "能用的 channel 型源照常复制"},
		{"channel shape problem", channel(protocol.OpenAIChat, "ftp://example.test"), 200, "", "channel 里 shape 有问题也不挡复制：复制一份再改仍是可走的路径"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := writeChannelConfig(t, t.TempDir(), `{"version":2,"profiles":{"src":`+tc.source+`}}`)
			w := callMgmt(NewMgmtRouter(), http.MethodPost, "/api/profiles/src/duplicate", `{"as":"copy"}`)
			if w.Code != tc.wantCode {
				t.Fatalf("duplicate 源为 %s = %d (%s), want %d；%s", tc.name, w.Code, strings.TrimSpace(w.Body.String()), tc.wantCode, tc.note)
			}
			if tc.wantMsg != "" && !strings.Contains(w.Body.String(), tc.wantMsg) {
				t.Fatalf("duplicate 源为 %s 的拒绝文案 = %s, want 含 %q", tc.name, strings.TrimSpace(w.Body.String()), tc.wantMsg)
			}
			raw, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatal(err)
			}
			if copied := strings.Contains(string(raw), `"copy"`); copied != (tc.wantCode == 200) {
				t.Fatalf("落盘与响应不一致：copy 存在=%v，响应码=%d（%s）", copied, w.Code, raw)
			}
		})
	}
}

// duplicate 与整文件门对「顶层 mode 不兼容、但 channel 覆盖它」这一格必须给出同一个结果。
// 主矩阵刻意不扩成 shape × api × responsesMode × channelMode（那是组合爆炸），所以这一格单独钉：
// 两格就是全部需要说的话——不可执行的顶层组合谁也放不过，合法的 channel override 谁都不许挡。
func TestDoorParity_DuplicateMatchesWholeFileTopLevelModeRule(t *testing.T) {
	legacy := func(mode string) string {
		return `{"version":2,"profiles":{"p":{"api":"openai-completions","responsesMode":"` + mode + `",` +
			`"baseUrl":"https://example.test/v1","apiKey":"k",` +
			`"upstreams":[{"name":"main","api":"openai-responses","responsesMode":"passthrough",` +
			`"baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}}}`
	}

	cases := []struct {
		name     string
		mode     string
		wantCode int
		note     string
	}{
		{"invalid top level with valid channel override", "passthrough", 400, "顶层组合自身不可执行：channel 覆盖它不豁免"},
		{"valid top level with valid channel override", "auto", 200, "channel override 本身合法，不得被当成错误"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := legacy(tc.mode)
			r := NewMgmtRouter()

			writeChannelConfig(t, t.TempDir(), `{"version":2,"profiles":{}}`)
			req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("PUT /api/config = %d (%s), want %d；%s", w.Code, strings.TrimSpace(w.Body.String()), tc.wantCode, tc.note)
			}

			// duplicate 的源就是盘上同一份 profile，所以两个门判的是同一格。
			writeChannelConfig(t, t.TempDir(), body)
			dup := callMgmt(r, http.MethodPost, "/api/profiles/p/duplicate", `{"as":"copy"}`)
			if dup.Code != tc.wantCode {
				t.Fatalf("duplicate = %d (%s), want %d（必须与整文件门一致）；%s", dup.Code, strings.TrimSpace(dup.Body.String()), tc.wantCode, tc.note)
			}
		})
	}
}
