package profile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/heihei0299/pi-switch/internal/config"
)

// CreateProfile is the single implementation behind POST /api/profiles and
// `pi-switch provider add`: it validates the profile (responsesMode
// compatibility, shape, retry knobs) and refuses to overwrite an existing
// supplier. Callers map its errors to their own surface (400/500 for HTTP,
// exit code + stderr for the CLI).
func CreateProfile(name string, prof config.ProviderProfile) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name required")
	}
	if err := ValidateProfile(prof); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.ProviderProfile{}
	}
	if _, exists := cfg.Profiles[name]; exists {
		return profileErr(ErrProfileExists, "profile already exists")
	}
	// 不在这里默认暴露全部：新建供应商的模型默认不暴露，需显式 expose，
	// 与"空 exposed = 不暴露"一致。
	cfg.Profiles[name] = prof
	return persist(cfg, "")
}

// DuplicateProfile copies a profile under a new name. It is the single
// implementation behind POST /api/profiles/:name/duplicate and
// `pi-switch provider duplicate`, and it never overwrites an existing supplier.
func DuplicateProfile(src, as string) error {
	as = strings.TrimSpace(as)
	if as == "" {
		// 服务端文案，不是 CLI 措辞：`--as <new>` 的提示由 CLI 自己给，
		// 核心函数不知道调用方是 HTTP 还是命令行。
		return ErrTargetNameRequired
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	prof, ok := cfg.Profiles[src]
	if !ok {
		return profileErr(ErrProfileNotFound, "profile %q not found", src)
	}
	if _, exists := cfg.Profiles[as]; exists {
		return profileErr(ErrProfileExists, "target %q already exists", as)
	}
	// 复制不校验内容，但 flat profile 的 api 就是它的 effective pair：把磁盘上一条能力上跑不
	// 起来的 legacy profile（遗留配置或手工编辑，三个授权门已经拒收这个形状）再复制一份，等于
	// 让 CRUD 重新产出必失败配置。只补这一格——shape/retry 仍然不判，否则「复制一份再改」这条
	// 修复路径会被 shape 问题一起挡掉。
	if len(prof.Upstreams) == 0 {
		if err := config.ValidateEffectiveFlatAPI(prof); err != nil {
			return err
		}
	}
	cfg.Profiles[as] = prof
	return persist(cfg, "failed to save config: ")
}

// SetExposedModels replaces the exposed model list of one channel. It is the
// single implementation behind PUT /api/profiles/:name/expose and
// `pi-switch provider expose`, and it refuses to expose an id that the channel
// does not actually carry (which would make routing claim a model it cannot
// serve).
func SetExposedModels(name, channel string, modelIDs []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		return profileErr(ErrProfileNotFound, "profile %q not found", name)
	}
	if channel == "" {
		return errors.New("channel is required")
	}
	idx := EnsureMutationChannel(&prof, channel)
	if idx < 0 {
		return profileErr(ErrUnknownChannel, "unknown channel %q", channel)
	}
	seen := map[string]bool{}
	for _, m := range prof.Upstreams[idx].Models {
		seen[m.ID] = true
	}
	for _, eid := range modelIDs {
		if !seen[eid] {
			return fmt.Errorf("exposedModels references unknown model %q in channel %q", eid, channel)
		}
	}
	prof.Upstreams[idx].ExposedModels = modelIDs
	cfg.Profiles[name] = prof
	return persist(cfg, "failed to save config: ")
}
