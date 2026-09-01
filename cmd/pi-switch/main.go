package main

import (
	"fmt"
	"os"

	"github.com/heihei0299/pi-switch/internal/server"
)

const version = "20260831.1.0-go"

func printHelp() {
	fmt.Printf(`pi-switch %s — Lightweight profile switcher for pi (Go rewrite)

Usage:
  pi-switch [command] [options]

Commands:
  proxy   Start proxy server (gateway)
  webui   Start WebUI server
  doctor  Run diagnostics
  help    Show help

Options:
  -h, --help     Show help
  -v, --version  Show version
  --host <host>  Host to bind (proxy/webui)
  --port <port>  Port to bind

Examples:
  pi-switch proxy start --daemon
  pi-switch webui start --daemon
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
		if len(args) > 1 && (args[1] == "-h" || args[1] == "--help" || args[1] == "help") {
			fmt.Println("Usage: pi-switch proxy [start] [--host HOST] [--port PORT] [--daemon]")
			os.Exit(0)
		}
		host := "127.0.0.1"
		port := "43112"
		for i := 1; i < len(args); i++ {
			if args[i] == "--host" && i+1 < len(args) {
				host = args[i+1]
				i++
			} else if args[i] == "--port" && i+1 < len(args) {
				port = args[i+1]
				i++
			}
		}
		r := server.NewProxyRouter()
		addr := host + ":" + port
		fmt.Printf("Proxy server listening on http://%s\n", addr)
		if err := r.Run(addr); err != nil {
			fmt.Fprintf(os.Stderr, "proxy: %v\n", err)
			os.Exit(1)
		}
	case "webui":
		if len(args) > 1 && (args[1] == "-h" || args[1] == "--help" || args[1] == "help") {
			fmt.Println("Usage: pi-switch webui [start] [--host HOST] [--port PORT] [--daemon]")
			os.Exit(0)
		}
		host := "127.0.0.1"
		port := "43110"
		for i := 1; i < len(args); i++ {
			if args[i] == "--host" && i+1 < len(args) {
				host = args[i+1]
				i++
			} else if args[i] == "--port" && i+1 < len(args) {
				port = args[i+1]
				i++
			}
		}
		r := server.NewMgmtRouter()
		addr := host + ":" + port
		fmt.Printf("WebUI server listening on http://%s\n", addr)
		if err := r.Run(addr); err != nil {
			fmt.Fprintf(os.Stderr, "webui: %v\n", err)
			os.Exit(1)
		}
	case "doctor":
		fmt.Println("doctor: ok (placeholder for 09)")
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printHelp()
		os.Exit(1)
	}
}
