package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/daemon"
	"github.com/heihei0299/pi-switch/internal/gateway"
	"github.com/heihei0299/pi-switch/internal/server"
	"github.com/heihei0299/pi-switch/internal/tui"
)

var (
	// Version is the single source of truth in package.json and is injected
	// at build time via ldflags. The remaining fields make a binary auditable
	// without relying on a package manager or an external build manifest.
	version     = "dev"
	buildTime   = "unknown"
	buildCommit = "unknown"
	buildTarget = "unknown"
	buildDirty  = "unknown"
)

func init() {
	server.Version = version
	server.BuildTime = buildTime
	server.BuildCommit = buildCommit
	server.BuildTarget = buildTarget
	server.BuildDirty = buildDirty
}

func printHelp() {
	fmt.Printf(`pi-switch %s — Lightweight profile switcher for pi (Go rewrite)

Usage:
  pi-switch [command] [options]

Commands:
  proxy     Start proxy server (gateway) — proxy start/stop/status [--host HOST] [--port PORT] [--daemon] [--generate-password]
  webui     Start WebUI server — webui start/stop/status [--host HOST] [--port PORT] [--daemon] [--generate-password]
  tui       Terminal UI (bubbletea) — profile list/switch, gateway status, stats
  provider  Manage suppliers — list | show <name> | add <name> [--preset P] [--api-key K] [--base-url U] [--models M1,M2] | duplicate <name> --as <new> | test <name> | fetch-models <name> [--channel C] | expose <name> <model-id>... [--channel C] | use <name> | delete <name> (aliases: ls, rm, remove)
  package   Package management — list | add <spec> [--disabled] | import | show <id> | delete <id>
  ccs       cc-switch — list (import is not implemented)
  presets   List presets — presets [list] | presets show <id>
  gateway   Gateway — gateway publish | gateway status | gateway preview
  stats     Not implemented — request stats live in the WebUI and GET /api/stats
  config    Config — config show | config validate | config path
  doctor    Run diagnostics
  build-info Show embedded build identity as JSON
  help      Show help

Options:
  -h, --help     Show help
  -v, --version  Show version
  --build-info   Show embedded build identity
  --host <host>  Host to bind (proxy/webui)
  --port <port>  Port to bind

Examples:
  pi-switch proxy start --daemon
  pi-switch webui start --daemon
  pi-switch tui
  pi-switch provider list
  pi-switch package list
  pi-switch ccs list
  pi-switch doctor

`, version)
}

func printBuildInfo() {
	payload := map[string]string{
		"version":   version,
		"buildTime": buildTime,
		"commit":    buildCommit,
		"target":    buildTarget,
		"dirty":     buildDirty,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-info: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printHelp()
		os.Exit(0)
	}
	switch args[0] {
	case "--", "-h", "--help", "help":
		printHelp()
		os.Exit(0)
	case "-v", "--version", "version":
		fmt.Println(version)
		os.Exit(0)
	case "proxy":
		handleProxy(args[1:])
	case "webui":
		handleWebUI(args[1:])
	case "tui":
		if len(args) > 1 && (args[1] == "-h" || args[1] == "--help") {
			fmt.Println("Usage: pi-switch tui  — launch bubbletea TUI (profile switch, gateway publish, stats)")
			os.Exit(0)
		}
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "tui: %v\n", err)
			os.Exit(1)
		}
	case "provider", "providers":
		os.Exit(handleProvider(args[1:]))
	case "package", "packages":
		os.Exit(handlePackage(args[1:]))
	case "ccs", "ccswitch", "cc-switch":
		os.Exit(handleCcs(args[1:]))
	case "presets", "preset":
		os.Exit(handlePresets(args[1:]))
	case "gateway":
		handleGatewayCLI(args[1:])
	case "stats":
		os.Exit(handleStatsCLI())
	case "config":
		os.Exit(handleConfigCLI(args[1:]))
	case "doctor":
		os.Exit(handleDoctor())
	case "--build-info", "build-info":
		printBuildInfo()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printHelp()
		os.Exit(1)
	}
}

func parseHostPort(args []string, defHost string, defPort int) (string, int, bool) {
	host := defHost
	port := defPort
	daemonFlag := false
	for i := 0; i < len(args); i++ {
		if args[i] == "--host" && i+1 < len(args) {
			host = args[i+1]
			i++
		} else if args[i] == "--port" && i+1 < len(args) {
			if p, err := strconv.Atoi(args[i+1]); err == nil {
				port = p
			}
			i++
		} else if args[i] == "--daemon" {
			daemonFlag = true
		}
	}
	return host, port, daemonFlag
}

func hasGeneratePasswordFlag(args []string) bool {
	for _, a := range args {
		if a == "--generate-password" {
			return true
		}
	}
	return false
}

// startWebUI is the single launch path for the management server. The direct
// run and the daemon child both arrive here, so the bind-address guard cannot
// be bypassed by spawning.
func startWebUI(host string, port int) {
	r := server.NewMgmtRouterWithAuth(mustResolveBindAuth(host))
	server.ImportLegacyOnStartup()
	addr := host + ":" + strconv.Itoa(port)
	fmt.Printf("WebUI server listening on http://%s\n", addr)
	if err := r.Run(addr); err != nil {
		fmt.Fprintf(os.Stderr, "webui: %v\n", err)
		os.Exit(1)
	}
}

// resolveBindAuth applies the bind-address guard for a listener about to start.
// Both the direct run and the daemon spawn call it, so an exposed proxy cannot
// start without a credential and the parent reports the same verdict the child
// will reach.
func mustResolveBindAuth(host string) server.MgmtAuthOptions {
	auth, err := server.ResolveAuthOptions(host, hasGeneratePasswordFlag(os.Args), func(msg string) {
		fmt.Println(msg)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	return auth
}

// startProxy is the single launch path for the proxy.
func startProxy(host string, port int) {
	r := server.NewProxyRouterWithAuth(mustResolveBindAuth(host))
	server.ImportLegacyOnStartup()
	addr := host + ":" + strconv.Itoa(port)
	fmt.Printf("Proxy server listening on http://%s\n", addr)
	if err := r.Run(addr); err != nil {
		fmt.Fprintf(os.Stderr, "proxy: %v\n", err)
		os.Exit(1)
	}
}

func startWebUIMode(host string, port int, isDaemon bool) {
	if isDaemon {
		// Resolve before spawning so an exposed bind without credentials fails
		// here, and so a generated password already exists for the child.
		mustResolveBindAuth(host)
		res, err := daemon.Start(daemon.WebUI, host, uint16(port))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Println(res.Message)
		os.Exit(0)
	}
	startWebUI(host, port)
}

func handleProxy(args []string) {
	if len(args) == 0 {
		args = []string{"start"}
	}
	sub := args[0]
	rest := []string{}
	if len(args) > 1 {
		rest = args[1:]
	}
	switch sub {
	case "-h", "--help", "help":
		fmt.Println("Usage: pi-switch proxy [start|stop|status] [--host HOST] [--port PORT] [--daemon] [--generate-password]")
		os.Exit(0)
	case "start":
		host, port, isDaemon := parseHostPort(rest, "127.0.0.1", 43112)
		if isDaemon {
			mustResolveBindAuth(host)
			res, err := daemon.Start(daemon.Proxy, host, uint16(port))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			fmt.Println(res.Message)
			// hint for multi-instance
			if res.Running {
				fmt.Println("hint: multi-instance check via `ss -tlnp | grep :" + strconv.Itoa(port) + "` if needed")
			}
			os.Exit(0)
		}
		startProxy(host, port)
	case "stop":
		res, _ := daemon.Stop(daemon.Proxy)
		fmt.Println(res.Message)
	case "status":
		res, _ := daemon.Status(daemon.Proxy)
		fmt.Println(res.Message)
		if !res.Running {
			os.Exit(1)
		}
	default:
		// treat as start with flags directly: pi-switch proxy --daemon etc
		host, port, isDaemon := parseHostPort(args, "127.0.0.1", 43112)
		if isDaemon {
			mustResolveBindAuth(host)
			res, err := daemon.Start(daemon.Proxy, host, uint16(port))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			fmt.Println(res.Message)
			os.Exit(0)
		}
		startProxy(host, port)
	}
}

func handleWebUI(args []string) {
	if len(args) == 0 {
		args = []string{"start"}
	}
	sub := args[0]
	rest := []string{}
	if len(args) > 1 {
		rest = args[1:]
	}
	switch sub {
	case "-h", "--help", "help":
		fmt.Println("Usage: pi-switch webui [start|stop|status] [--host HOST] [--port PORT] [--daemon] [--generate-password]")
		os.Exit(0)
	case "start":
		host, port, isDaemon := parseHostPort(rest, "127.0.0.1", 43110)
		startWebUIMode(host, port, isDaemon)
	case "stop":
		res, _ := daemon.Stop(daemon.WebUI)
		fmt.Println(res.Message)
	case "status":
		res, _ := daemon.Status(daemon.WebUI)
		fmt.Println(res.Message)
		if !res.Running {
			os.Exit(1)
		}
	default:
		host, port, isDaemon := parseHostPort(args, "127.0.0.1", 43110)
		startWebUIMode(host, port, isDaemon)
	}
}

// providerAddFlags carries the knobs `provider add` accepts. Values are taken
// from flags rather than an interactive picker so the command stays scriptable.
type providerAddFlags struct {
	preset  string
	apiKey  string
	baseURL string
	api     string
	models  []string
}

// modelEntries turns CLI model ids into the config shape; context/maxTokens are
// left to the catalog enricher (same as the WebUI, where a bare id is normal).
func modelEntries(ids []string) []config.ModelEntry {
	out := make([]config.ModelEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, config.ModelEntry{ID: id})
	}
	return out
}

// scanSubcommandFlags splits one provider subcommand's arguments into positional
// values and `--flag value` pairs.
//
// Unknown flags, flags without a value and repeated flags are all refused: the
// three subcommands that need this used to hand-roll the loop with three
// different levels of strictness, so a typo could become a model id or be
// dropped without a word.
func scanSubcommandFlags(sub string, args []string, allowed ...string) ([]string, map[string]string, error) {
	known := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		known[a] = true
	}
	var positional []string
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		if !known[a] {
			return nil, nil, fmt.Errorf("%s: unknown flag %q", sub, a)
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("%s: %s requires a value", sub, a)
		}
		if _, dup := flags[a]; dup {
			return nil, nil, fmt.Errorf("%s: %s given more than once", sub, a)
		}
		flags[a] = args[i+1]
		i++
	}
	return positional, flags, nil
}

// parseProviderAddArgs expects exactly one positional name; unknown flags are
// rejected rather than ignored, so a typo cannot create an unintended profile.
func parseProviderAddArgs(args []string) (string, providerAddFlags, error) {
	flags := providerAddFlags{api: "openai-responses"}
	name := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		takeValue := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("provider add: %s requires a value", a)
			}
			i++
			return args[i], nil
		}
		switch a {
		case "--preset":
			v, err := takeValue()
			if err != nil {
				return "", flags, err
			}
			flags.preset = v
		case "--api-key":
			v, err := takeValue()
			if err != nil {
				return "", flags, err
			}
			flags.apiKey = v
		case "--base-url":
			v, err := takeValue()
			if err != nil {
				return "", flags, err
			}
			flags.baseURL = v
		case "--api":
			v, err := takeValue()
			if err != nil {
				return "", flags, err
			}
			flags.api = v
		case "--models":
			v, err := takeValue()
			if err != nil {
				return "", flags, err
			}
			for _, id := range strings.Split(v, ",") {
				if id = strings.TrimSpace(id); id != "" {
					flags.models = append(flags.models, id)
				}
			}
		default:
			if strings.HasPrefix(a, "-") {
				return "", flags, fmt.Errorf("provider add: unknown flag %q", a)
			}
			if name != "" {
				return "", flags, fmt.Errorf("provider add takes one name, got %q and %q", name, a)
			}
			name = a
		}
	}
	if strings.TrimSpace(name) == "" {
		return "", flags, fmt.Errorf("provider add <name> required")
	}
	return name, flags, nil
}

func handleProvider(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(`Usage: pi-switch provider <list|show|add|duplicate|test|fetch-models|expose|use|delete> [name]
  aliases        ls = list, rm = remove = delete
  add            <name> [--preset P] [--api-key K] [--base-url U] [--api M] [--models M1,M2]
  duplicate      <name> --as <new>
  test           <name>
  fetch-models   <name> [--channel C]   (with --channel: enrich and merge into that channel)
  expose         <name> <model-id>... [--channel C]   (--channel required for a multi-channel profile)

`)
		return 0
	}
	cfgPath := config.ResolvePath()
	cfg, _, _ := config.LoadConfigAtPath(cfgPath)
	switch args[0] {
	case "list", "ls":
		if len(cfg.Profiles) == 0 {
			fmt.Println("No profiles.")
			return 0
		}
		for name := range cfg.Profiles {
			mark := " "
			if cfg.Current != nil && *cfg.Current == name {
				mark = "*"
			}
			fmt.Printf("%s %s\n", mark, name)
		}
	case "add":
		// 与服务端 POST /api/profiles 共用 CreateProfile，校验与落盘只有一份实现。
		name, flags, err := parseProviderAddArgs(args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		prof := config.ProviderProfile{API: flags.api, BaseURL: flags.baseURL, APIKey: flags.apiKey}
		if flags.preset != "" {
			prof.Preset = &flags.preset
		}
		// 顶层 baseUrl 不构成可路由的渠道：路由与 expose 都以 upstreams[] 为单位。
		// 给了 --base-url 就同时建一个名为 main 的渠道，使新增的供应商立刻可用
		// （fetch-models/expose 都需要渠道）。
		if flags.baseURL != "" {
			channel := "main"
			prof.Upstreams = []config.Upstream{{
				Name:          &channel,
				API:           flags.api,
				BaseURL:       flags.baseURL,
				APIKey:        flags.apiKey,
				Models:        modelEntries(flags.models),
				ExposedModels: []string{},
			}}
		}
		if err := server.CreateProfile(name, prof); err != nil {
			fmt.Fprintf(os.Stderr, "provider add failed: %v\n", err)
			return 1
		}
		fmt.Printf("Added %s\n", name)
	case "duplicate":
		positional, flags, err := scanSubcommandFlags("provider duplicate", args[1:], "--as")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		if len(positional) != 1 || flags["--as"] == "" {
			fmt.Fprintln(os.Stderr, "provider duplicate <name> --as <new> required")
			return 1
		}
		src, as := positional[0], flags["--as"]
		if err := server.DuplicateProfile(src, as); err != nil {
			fmt.Fprintf(os.Stderr, "provider duplicate failed: %v\n", err)
			return 1
		}
		fmt.Printf("Duplicated %s as %s\n", src, as)
	case "test":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "provider test <name> required")
			return 1
		}
		prof, ok := cfg.Profiles[args[1]]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown profile %q\n", args[1])
			return 1
		}
		success, message, ms := server.TestProfileUpstream(prof)
		if !success {
			fmt.Fprintf(os.Stderr, "provider test %s: %s (%dms)\n", args[1], message, ms)
			return 1
		}
		fmt.Printf("%s: %s (%dms)\n", args[1], message, ms)
	case "fetch-models":
		positional, flags, err := scanSubcommandFlags("provider fetch-models", args[1:], "--channel")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		if len(positional) != 1 {
			fmt.Fprintln(os.Stderr, "provider fetch-models <name> [--channel <channel>] required")
			return 1
		}
		name := positional[0]
		prof, ok := cfg.Profiles[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown profile %q\n", name)
			return 1
		}
		// --channel 走渠道定向拉取（enrich 后合并入该渠道并落盘），与 handler 同一实现；
		// 不给渠道时保持只读列出，不写盘。
		if channel := flags["--channel"]; channel != "" {
			ids, counts, err := server.FetchChannelModels(name, channel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "provider fetch-models %s: %v\n", name, err)
				return 1
			}
			b, _ := json.Marshal(map[string]interface{}{
				"models": ids,
				"enrich": map[string]interface{}{"enriched": counts.Enriched, "skipped": counts.Skipped, "failed": counts.Failed},
			})
			fmt.Println(string(b))
			return 0
		}
		ids, lastErr := server.FetchUpstreamModelIDs(prof)
		if lastErr != "" {
			fmt.Fprintf(os.Stderr, "provider fetch-models %s: %s\n", name, lastErr)
			return 1
		}
		b, _ := json.Marshal(map[string]interface{}{"models": ids})
		fmt.Println(string(b))
	case "expose":
		positional, flags, err := scanSubcommandFlags("provider expose", args[1:], "--channel")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		// 至少一个 model id：`expose <name>` 曾把该渠道的 exposedModels 清空却报成功。
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "provider expose <name> <model-id>... [--channel <channel>] required")
			return 1
		}
		name, ids := positional[0], positional[1:]
		channel := flags["--channel"]
		prof, ok := cfg.Profiles[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown profile %q\n", name)
			return 1
		}
		switch {
		case channel != "":
			// 显式指定，交给核心校验渠道是否存在。
		case len(prof.Upstreams) == 1:
			// 单渠道 profile 无需显式指定，按唯一渠道推断。
			channel = prof.ChannelName(0)
		case len(prof.Upstreams) == 0:
			// 顶层 baseUrl 不构成渠道；旧形状的 profile 一个渠道都没有，
			// 不能告诉用户"渠道太多"。
			fmt.Fprintf(os.Stderr, "provider expose: profile %q has no channels; add one with a baseUrl before exposing\n", name)
			return 1
		default:
			fmt.Fprintln(os.Stderr, "provider expose: --channel <channel> required (profile has multiple channels)")
			return 1
		}
		if err := server.SetExposedModels(name, channel, ids); err != nil {
			fmt.Fprintf(os.Stderr, "provider expose failed: %v\n", err)
			return 1
		}
		fmt.Printf("Exposed %d model(s) on %s/%s\n", len(ids), name, channel)
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "provider show <name> required")
			return 1
		}
		prof, ok := cfg.Profiles[args[1]]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown profile %q\n", args[1])
			return 1
		}
		b, _ := json.MarshalIndent(prof, "", "  ")
		fmt.Println(string(b))
	case "use":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "provider use <name> required")
			return 1
		}
		name := args[1]
		if _, ok := cfg.Profiles[name]; !ok {
			fmt.Fprintf(os.Stderr, "unknown profile %q\n", name)
			return 1
		}
		cfg.Current = &name
		if err := saveConfigFile(cfg, cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "provider use failed to save config: %v\n", err)
			return 1
		}
		fmt.Printf("Switched to %s\n", name)
	case "delete", "remove", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "provider delete <name> required")
			return 1
		}
		delete(cfg.Profiles, args[1])
		if cfg.Current != nil && *cfg.Current == args[1] {
			cfg.Current = nil
		}
		if err := saveConfigFile(cfg, cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "provider delete failed to save config: %v\n", err)
			return 1
		}
		fmt.Printf("Deleted %s\n", args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown provider subcommand %q\n", args[0])
		return 1
	}
	return 0
}

// handlePackage reports failures by returning a non-zero exit code instead of
// printing a success payload, so scripts reading stdout cannot mistake a no-op
// for a completed operation.
func handlePackage(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch package <list|add|import|show|delete> [args]")
		fmt.Println("  add <spec> [--disabled]   spec is one token, e.g. npm:@scope/pkg or ./local-dir")
		return 0
	}
	switch args[0] {
	case "list", "ls":
		packages, err := server.ListInstalledPackages()
		if err != nil {
			fmt.Fprintf(os.Stderr, "package list failed: %v\n", err)
			return 1
		}
		b, _ := json.Marshal(map[string]interface{}{"packages": packages})
		fmt.Println(string(b))
	case "add":
		enabled := true
		var positionals []string
		for _, a := range args[1:] {
			if a == "--disabled" || a == "--no-enabled" {
				enabled = false
				continue
			}
			positionals = append(positionals, a)
		}
		if len(positionals) == 0 {
			fmt.Fprintln(os.Stderr, "package add <spec> required")
			return 1
		}
		// Discarding extra arguments silently would persist a record that does
		// not match what the operator asked for.
		if len(positionals) > 1 {
			fmt.Fprintf(os.Stderr, "package add takes one spec, got %d (%q); a spec is a single token such as npm:pkg or ./dir\n", len(positionals), positionals)
			return 1
		}
		spec := positionals[0]
		if err := server.AddInstalledPackage(spec, enabled); err != nil {
			fmt.Fprintf(os.Stderr, "package add failed: %v\n", err)
			return 1
		}
		fmt.Printf("{\"ok\":true,\"id\":%q}\n", strings.TrimSpace(spec))
	case "sync":
		// Importing pi packages is the only real sync this CLI has; do not
		// report success for a mode that does nothing.
		fmt.Fprintln(os.Stderr, `package sync is not implemented; use "pi-switch package import" to sync packages from the pi agent directory`)
		return 2
	case "import":
		result, err := server.ImportPiPackages()
		if err != nil {
			fmt.Fprintf(os.Stderr, "package import failed: %v\n", err)
			return 1
		}
		b, _ := json.Marshal(result)
		fmt.Println(string(b))
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "package show <id> required")
			return 1
		}
		m, err := server.GetInstalledPackage(args[1])
		if err != nil {
			if errors.Is(err, server.ErrPackageNotFound) {
				fmt.Fprintf(os.Stderr, "package %q not found\n", args[1])
				return 1
			}
			fmt.Fprintf(os.Stderr, "package show failed: %v\n", err)
			return 1
		}
		b, _ := json.MarshalIndent(m, "", "  ")
		fmt.Println(string(b))
	case "delete", "remove", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "package delete <id> required")
			return 1
		}
		if err := server.DeleteInstalledPackage(args[1]); err != nil {
			if errors.Is(err, server.ErrPackageNotFound) {
				fmt.Fprintf(os.Stderr, "package %q not found\n", args[1])
				return 1
			}
			fmt.Fprintf(os.Stderr, "package delete failed: %v\n", err)
			return 1
		}
		fmt.Printf("{\"ok\":true,\"id\":%q}\n", args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown package subcommand %q\n", args[0])
		return 1
	}
	return 0
}

func handleCcs(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch ccs <list|import> [options]")
		return 0
	}
	switch args[0] {
	case "list", "ls":
		// No CCS provider store exists, so the empty list is the real answer and
		// claims nothing happened.
		fmt.Println(`{"providers":[]}`)
	case "import":
		// Claiming "imported" without importing anything is the defect this
		// ticket removes; say so and fail instead.
		fmt.Fprintln(os.Stderr, `ccs import is not implemented: pi-switch can read neither the ccs configuration nor its provider store`)
		return 2
	default:
		fmt.Fprintf(os.Stderr, "unknown ccs subcommand %q\n", args[0])
		return 1
	}
	return 0
}

func handlePresets(args []string) int {
	var id string
	if len(args) > 0 {
		id = args[0]
	}
	switch id {
	case "-h", "--help", "list", "ls":
		// `presets list` is the documented spelling; treat it as the no-arg form.
		id = ""
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "preset show <id> required")
			return 1
		}
		id = args[1]
	}
	presets := server.ProviderPresets()
	if id == "" {
		b, _ := json.Marshal(presets)
		fmt.Println(string(b))
		return 0
	}
	for _, p := range presets {
		if p["id"] == id {
			b, _ := json.MarshalIndent(p, "", "  ")
			fmt.Println(string(b))
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "unknown preset %q\n", id)
	return 1
}

func handleStatsCLI() int {
	fmt.Fprintln(os.Stderr, `stats is not implemented in the CLI; the aggregated numbers live in the WebUI and in GET /api/stats while "pi-switch webui start" is running`)
	return 2
}

// handleDoctor probes real state. It exits non-zero only for conditions that are
// genuinely broken (unreadable config, unusable database, an exposed WebUI with
// no password); a missing config or a stopped daemon is reported as a fact, not
// a failure.
func handleDoctor() int {
	cfgPath := config.ResolvePath()
	problems := 0

	if _, err := os.Stat(cfgPath); err != nil {
		fmt.Printf("config: %s (not created yet)\n", cfgPath)
	} else if err := configFileProblem(cfgPath); err != nil {
		// LoadConfigAtPath tolerates unparsable JSON by returning defaults, so a
		// corrupt config would otherwise look healthy to this probe.
		fmt.Printf("config: %s unreadable: %v\n", cfgPath, err)
		problems++
	} else {
		// LoadConfigAtPath never returns a non-nil error today (it falls back to
		// defaults), so its source string is the honest diagnostic to report
		// rather than an error branch that can never run.
		cfg, source, _ := config.LoadConfigAtPath(cfgPath)
		fmt.Printf("config: %s (%d profiles, %s)\n", cfgPath, len(cfg.Profiles), source)
	}

	dbPath := server.PiSwitchDBPath()
	if _, err := os.Stat(dbPath); err != nil {
		fmt.Printf("database: %s (not created yet)\n", dbPath)
	} else if db, err := sql.Open("sqlite", dbPath); err != nil {
		fmt.Printf("database: %s unreadable: %v\n", dbPath, err)
		problems++
	} else {
		err = db.Ping()
		_ = db.Close()
		if err != nil {
			fmt.Printf("database: %s unreadable: %v\n", dbPath, err)
			problems++
		} else {
			fmt.Printf("database: %s readable\n", dbPath)
		}
	}

	proxyRes, _ := daemon.Status(daemon.Proxy)
	fmt.Printf("proxy daemon: %s\n", daemonSummary(proxyRes))
	// An already-running listener bound beyond loopback without a password is a
	// real exposure; anything else is just a fact to report.
	webRes, _ := daemon.Status(daemon.WebUI)
	if webRes.Running && !server.IsLoopback(derefString(webRes.Host)) && server.WebUIPasswordConfigured() == "" {
		fmt.Printf("webui daemon: running on %s with no password, so the management API is exposed\n", derefString(webRes.Host))
		problems++
	} else {
		fmt.Printf("webui daemon: %s\n", daemonSummary(webRes))
	}

	cap := server.ProxyBodyLimit()
	fmt.Printf("proxy body cap: %d bytes\n", cap)
	if raw := os.Getenv("PI_SWITCH_MAX_BODY_BYTES"); raw != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err != nil || n <= 0 {
			fmt.Printf("proxy body cap: PI_SWITCH_MAX_BODY_BYTES=%q is not a positive integer, so the default is in effect\n", raw)
		}
	}

	if problems > 0 {
		fmt.Fprintf(os.Stderr, "doctor: %d problem(s) found\n", problems)
		return 1
	}
	fmt.Println("doctor: ok")
	return 0
}

// daemonSummary reports the daemon state without printing pointer addresses and
// without discarding the explanation Status computed (it returns Running=false
// with a reason when the process is alive but its port does not answer).
func daemonSummary(res daemon.DaemonResult) string {
	if !res.Running {
		if res.Message != "" {
			return "not running: " + res.Message
		}
		return "not running"
	}
	if res.Pid != nil {
		return fmt.Sprintf("running (pid %d)", *res.Pid)
	}
	return "running"
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// configFileProblem reports whether an existing config file is unusable. The
// loader is deliberately tolerant, so the probe has to check the bytes itself.
func configFileProblem(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !json.Valid(b) {
		return errors.New("config file is not valid JSON")
	}
	return nil
}

// publishedProxyAuthWarning reports the authentication limitation of what is about
// to be published, or "" when there is nothing to say.
//
// The host comes from the entries themselves rather than from a second reading of
// the config: the gateway package already decides the baseUrl (including rewriting
// a wildcard host to loopback), and re-deriving it here would be a second copy
// free to drift. The limitation is that the published providers carry
// `"apiKey": "pi-switch-proxy"`, which a client sends as a Bearer token, while the
// proxy only accepts HTTP Basic once it is bound beyond loopback.
//
// No credential may appear in this text: warning is the whole point of not
// publishing the shared password into ~/.pi/agent/models.json.
func publishedProxyAuthWarning(published map[string]interface{}) string {
	providers, _ := published["providers"].(map[string]interface{})
	for _, raw := range providers {
		entry, _ := raw.(map[string]interface{})
		base, _ := entry["baseUrl"].(string)
		u, err := url.Parse(base)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if !server.IsLoopback(u.Hostname()) {
			return "the published providers point at " + u.Hostname() + " and carry a Bearer-style apiKey, but the proxy only accepts HTTP Basic authentication beyond loopback — such clients get 401. Bind the proxy to loopback, or use a Basic-capable client."
		}
	}
	return ""
}

func handleGatewayCLI(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch gateway <publish|status|preview>")
		os.Exit(0)
	}
	cfgPath := config.ResolvePath()
	cfg, _, _ := config.LoadConfigAtPath(cfgPath)
	switch args[0] {
	case "publish", "apply":
		toPublish := gateway.BuildProposedGatewayEntry(cfg)
		if err := gateway.Publish(cfg, toPublish); err != nil {
			fmt.Fprintf(os.Stderr, "gateway publish failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("gateway published")
		// 与 WebUI 那条路一致地告知限制：发布的 provider 携带 Bearer 形式的
		// apiKey，而代理面在超出 loopback 时只接受 HTTP Basic。判断取自**即将写入
		// 的条目**里的 baseUrl，不再自己推一遍 host，避免与 gateway 包的推法漂移。
		if warn := publishedProxyAuthWarning(toPublish); warn != "" {
			fmt.Fprintln(os.Stderr, "warning: "+warn)
		}
	case "status":
		fmt.Printf("Gateway @ %s:%d\n", cfg.Settings.Proxy.Host, cfg.Settings.Proxy.Port)
	case "preview":
		preview := gateway.BuildProposedGatewayEntry(cfg)
		b, _ := json.MarshalIndent(preview, "", "  ")
		fmt.Println(string(b))
	default:
		fmt.Fprintf(os.Stderr, "unknown gateway subcommand %q\n", args[0])
		os.Exit(1)
	}
}

func handleConfigCLI(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch config <show|validate|path>")
		return 0
	}
	cfgPath := config.ResolvePath()
	switch args[0] {
	case "show", "path":
		fmt.Println(cfgPath)
	case "validate":
		// A corrupt file must not be reported as valid: LoadConfigAtPath falls
		// back to a default config that has a placeholder profile, so checking
		// the parsed result alone would always look healthy.
		if err := configFileProblem(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "config invalid: %v\n", err)
			return 1
		}
		cfg, _, _ := config.LoadConfigAtPath(cfgPath)
		if len(cfg.Profiles) == 0 {
			fmt.Println("warning: no profiles")
		} else {
			fmt.Println("config valid")
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n", args[0])
		return 1
	}
	return 0
}

func saveConfigFile(cfg config.PiSwitchConfig, path string) error {
	return config.SaveAtPath(cfg, path)
}
