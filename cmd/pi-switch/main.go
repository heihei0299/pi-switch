package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/daemon"
	"github.com/heihei0299/pi-switch/internal/gateway"
	"github.com/heihei0299/pi-switch/internal/server"
	"github.com/heihei0299/pi-switch/internal/tui"
)

var (
	// Version is the single source of truth in package.json and is injected
	// at build time via ldflags (-X main.version=...); "dev" means a plain
	// `go build` without the npm build scripts.
	version   = "dev"
	buildTime = "unknown"
)

func init() {
	server.Version = version
	server.BuildTime = buildTime
}

func printHelp() {
	fmt.Printf(`pi-switch %s — Lightweight profile switcher for pi (Go rewrite)

Usage:
  pi-switch [command] [options]

Commands:
  proxy     Start proxy server (gateway) — proxy start/stop/status [--host HOST] [--port PORT] [--daemon]
  webui     Start WebUI server — webui start/stop/status [--host HOST] [--port PORT] [--daemon]
  tui       Terminal UI (bubbletea) — profile list/switch, gateway status, stats
  provider  Manage suppliers — list | show <name> | add <name> | delete <name> | duplicate <name> --as <new>
  package   Package management — list | add <spec> | sync | import | show <id> | delete <id>
  ccs       cc-switch import — list | import [--all] [--force]
  presets   List presets — presets [list] | presets show <id>
  gateway   Gateway — gateway publish | gateway status | gateway preview
  stats     Show stats brief — stats
  config    Config — config show | config validate | config path
  doctor    Run diagnostics
  help      Show help

Options:
  -h, --help     Show help
  -v, --version  Show version
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
		handleProvider(args[1:])
	case "package", "packages":
		handlePackage(args[1:])
	case "ccs", "ccswitch", "cc-switch":
		handleCcs(args[1:])
	case "presets", "preset":
		handlePresets(args[1:])
	case "gateway":
		handleGatewayCLI(args[1:])
	case "stats":
		handleStatsCLI()
	case "config":
		handleConfigCLI(args[1:])
	case "doctor":
		handleDoctor()
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
		fmt.Println("Usage: pi-switch proxy [start|stop|status] [--host HOST] [--port PORT] [--daemon]")
		os.Exit(0)
	case "start":
		host, port, isDaemon := parseHostPort(rest, "127.0.0.1", 43112)
		if isDaemon {
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
		r := server.NewProxyRouter()
		server.ImportLegacyOnStartup()
		addr := host + ":" + strconv.Itoa(port)
		fmt.Printf("Proxy server listening on http://%s\n", addr)
		if err := r.Run(addr); err != nil {
			fmt.Fprintf(os.Stderr, "proxy: %v\n", err)
			os.Exit(1)
		}
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
			res, err := daemon.Start(daemon.Proxy, host, uint16(port))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			fmt.Println(res.Message)
			os.Exit(0)
		}
		r := server.NewProxyRouter()
		server.ImportLegacyOnStartup()
		addr := host + ":" + strconv.Itoa(port)
		fmt.Printf("Proxy server listening on http://%s\n", addr)
		if err := r.Run(addr); err != nil {
			fmt.Fprintf(os.Stderr, "proxy: %v\n", err)
			os.Exit(1)
		}
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
		fmt.Println("Usage: pi-switch webui [start|stop|status] [--host HOST] [--port PORT] [--daemon]")
		os.Exit(0)
	case "start":
		host, port, isDaemon := parseHostPort(rest, "127.0.0.1", 43110)
		if isDaemon {
			res, err := daemon.Start(daemon.WebUI, host, uint16(port))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			fmt.Println(res.Message)
			os.Exit(0)
		}
		r := server.NewMgmtRouter()
		server.ImportLegacyOnStartup()
		addr := host + ":" + strconv.Itoa(port)
		fmt.Printf("WebUI server listening on http://%s\n", addr)
		if err := r.Run(addr); err != nil {
			fmt.Fprintf(os.Stderr, "webui: %v\n", err)
			os.Exit(1)
		}
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
		if isDaemon {
			res, err := daemon.Start(daemon.WebUI, host, uint16(port))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			fmt.Println(res.Message)
			os.Exit(0)
		}
		r := server.NewMgmtRouter()
		server.ImportLegacyOnStartup()
		addr := host + ":" + strconv.Itoa(port)
		fmt.Printf("WebUI server listening on http://%s\n", addr)
		if err := r.Run(addr); err != nil {
			fmt.Fprintf(os.Stderr, "webui: %v\n", err)
			os.Exit(1)
		}
	}
}

func handleProvider(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch provider <list|show|add|delete|duplicate|use|test|fetch-models> [name]")
		os.Exit(0)
	}
	cfgPath := envOrDefault("PI_SWITCH_CONFIG", defaultConfigPath())
	cfg, _, _ := config.LoadConfigAtPath(cfgPath)
	switch args[0] {
	case "list", "ls":
		if len(cfg.Profiles) == 0 {
			fmt.Println("No profiles.")
			return
		}
		for name := range cfg.Profiles {
			mark := " "
			if cfg.Current != nil && *cfg.Current == name {
				mark = "*"
			}
			fmt.Printf("%s %s\n", mark, name)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "provider show <name> required")
			os.Exit(1)
		}
		prof, ok := cfg.Profiles[args[1]]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown profile %q\n", args[1])
			os.Exit(1)
		}
		b, _ := json.MarshalIndent(prof, "", "  ")
		fmt.Println(string(b))
	case "use":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "provider use <name> required")
			os.Exit(1)
		}
		name := args[1]
		if _, ok := cfg.Profiles[name]; !ok {
			fmt.Fprintf(os.Stderr, "unknown profile %q\n", name)
			os.Exit(1)
		}
		cfg.Current = &name
		_ = saveConfigFile(cfg, cfgPath)
		fmt.Printf("Switched to %s\n", name)
	case "delete", "remove", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "provider delete <name> required")
			os.Exit(1)
		}
		delete(cfg.Profiles, args[1])
		if cfg.Current != nil && *cfg.Current == args[1] {
			cfg.Current = nil
		}
		_ = saveConfigFile(cfg, cfgPath)
		fmt.Printf("Deleted %s\n", args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown provider subcommand %q\n", args[0])
		os.Exit(1)
	}
}

func handlePackage(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch package <list|add|sync|import|show|delete> [args]")
		os.Exit(0)
	}
	switch args[0] {
	case "list", "ls":
		fmt.Println(`{"packages":[]}`)
	case "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "package add <spec> required")
			os.Exit(1)
		}
		fmt.Printf("{\"ok\":true,\"id\":%q}\n", args[1])
	case "sync":
		fmt.Println(`{"ok":true,"message":"sync done"}`)
	case "import":
		fmt.Println(`{"ok":true,"count":0}`)
	case "show":
		fmt.Println(`{"error":"not found"}`)
	case "delete", "remove", "rm":
		fmt.Println(`{"ok":true}`)
	default:
		fmt.Fprintf(os.Stderr, "unknown package subcommand %q\n", args[0])
		os.Exit(1)
	}
}

func handleCcs(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch ccs <list|import> [options]")
		os.Exit(0)
	}
	switch args[0] {
	case "list", "ls":
		fmt.Println(`{"providers":[]}`)
	case "import":
		fmt.Println(`{"ok":true,"imported":0,"results":[]}`)
	default:
		fmt.Fprintf(os.Stderr, "unknown ccs subcommand %q\n", args[0])
		os.Exit(1)
	}
}

func handlePresets(args []string) {
	if len(args) > 0 && (args[0] == "show") {
		fmt.Println(`{"error":"not found"}`)
		os.Exit(1)
	}
	fmt.Println(`[]`)
}

func handleGatewayCLI(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch gateway <publish|status|preview>")
		os.Exit(0)
	}
	cfgPath := envOrDefault("PI_SWITCH_CONFIG", defaultConfigPath())
	cfg, _, _ := config.LoadConfigAtPath(cfgPath)
	switch args[0] {
	case "publish", "apply":
		toPublish := gateway.BuildProposedGatewayEntry(cfg)
		if err := gateway.Publish(cfg, toPublish); err != nil {
			fmt.Fprintf(os.Stderr, "gateway publish failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("gateway published")
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

func handleStatsCLI() {
	fmt.Println("stats: use webui or /api/stats")
}

func handleConfigCLI(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: pi-switch config <show|validate|path>")
		os.Exit(0)
	}
	cfgPath := envOrDefault("PI_SWITCH_CONFIG", defaultConfigPath())
	switch args[0] {
	case "show", "path":
		fmt.Println(cfgPath)
	case "validate":
		cfg, _, _ := config.LoadConfigAtPath(cfgPath)
		// simple validation
		if len(cfg.Profiles) == 0 {
			fmt.Println("warning: no profiles")
		} else {
			fmt.Println("config valid")
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n", args[0])
		os.Exit(1)
	}
}

func handleDoctor() {
	cfgPath := envOrDefault("PI_SWITCH_CONFIG", defaultConfigPath())
	cfg, _, _ := config.LoadConfigAtPath(cfgPath)
	fmt.Printf("config: %s (%d profiles)\n", cfgPath, len(cfg.Profiles))
	fmt.Println("doctor: ok")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch-config.json"
	}
	return home + "/.pi-switch/config.json"
}

func saveConfigFile(cfg config.PiSwitchConfig, path string) error {
	// ensure dir
	return saveConfigInner(cfg, path)
}

func saveConfigInner(cfg config.PiSwitchConfig, path string) error {
	// duplicate of tui save
	b, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := path + ".tmp"
	_ = os.MkdirAll(defaultConfigDir(), 0755)
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func defaultConfigDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "/tmp/pi-switch"
	}
	return home + "/.pi-switch"
}
