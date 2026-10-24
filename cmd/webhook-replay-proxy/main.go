package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/smford/webhook-replay-proxy/internal/config"
	"github.com/smford/webhook-replay-proxy/internal/proxy"
	"github.com/smford/webhook-replay-proxy/internal/store"
)

var (
	version = "0.1.0"
	commit  = "none"
	date    = "unknown"
)

type arrayFlags []string

func (a *arrayFlags) String() string {
	return strings.Join(*a, ", ")
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func printVersion() {
	fmt.Printf("webhook-replay-proxy %s (commit: %s, date: %s)\n", version, commit, date)
	fmt.Println("Copyright (c) 2026 smford")
	fmt.Println("Licensed under the MIT License")
	fmt.Println("https://github.com/smford/webhook-replay-proxy")
}

func printUsage() {
	printVersion()
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  webhook-replay-proxy [command] [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  listen             Start webhook capture and forwarding server (default)")
	fmt.Println("  list               List recorded webhooks from SQLite database")
	fmt.Println("  inspect <event_id> Inspect recorded webhook payload, headers, and replay history")
	fmt.Println("  replay <event_id>  Replay a captured webhook with mutations and HMAC re-signing")
	fmt.Println("  curl <event_id>    Generate ready-to-run curl command with fresh HMAC signature")
	fmt.Println("  version            Show version information")
	fmt.Println()
	fmt.Println("Flags for 'listen':")
	fmt.Println("  --port, -port <int>        Listen port (default: 8080)")
	fmt.Println("  --forward-to <url>         Forward incoming webhooks to target URL")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println("  --config <path>            Path to YAML configuration file")
	fmt.Println()
	fmt.Println("Flags for 'list':")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println("  --limit <int>              Maximum events to list (default: 50)")
	fmt.Println()
	fmt.Println("Flags for 'inspect':")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println("  --raw                      Print raw unformatted payload body")
	fmt.Println()
	fmt.Println("Flags for 'replay':")
	fmt.Println("  --target <url>             Override destination target URL")
	fmt.Println("  --provider <name>          Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fmt.Println("  --secret <string>          Webhook HMAC secret to re-sign payload with fresh timestamp")
	fmt.Println("  --set <key.path=value>     Mutate JSON field (can be specified multiple times)")
	fmt.Println("  --concurrency <int>        Number of concurrent requests fired simultaneously (default: 1)")
	fmt.Println("  --delay <duration>         Delay between spaced requests (e.g. 100ms, 1s)")
	fmt.Println("  --diff                     Display visual diff of mutated JSON payload before replaying")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println()
	fmt.Println("Flags for 'curl':")
	fmt.Println("  --target <url>             Override destination target URL")
	fmt.Println("  --provider <name>          Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fmt.Println("  --secret <string>          Webhook HMAC secret to re-sign payload with fresh timestamp")
	fmt.Println("  --set <key.path=value>     Mutate JSON field (can be specified multiple times)")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
}

func main() {
	args := os.Args[1:]

	// Check top-level version or help flags
	for _, arg := range args {
		if arg == "-v" || arg == "--version" || arg == "-version" || arg == "version" {
			printVersion()
			return
		}
		if arg == "-h" || arg == "--help" || arg == "help" {
			printUsage()
			return
		}
	}

	cmd := "listen"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "listen":
		runListen(args)
	case "list":
		runList(args)
	case "inspect":
		runInspect(args)
	case "replay":
		runReplay(args)
	case "curl":
		runCurl(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func runListen(args []string) {
	fs := flag.NewFlagSet("listen", flag.ExitOnError)
	var (
		port       int
		forwardTo  string
		dbPath     string
		configPath string
	)

	fs.IntVar(&port, "port", 8080, "Port to listen on")
	fs.StringVar(&forwardTo, "forward-to", "", "Forward target URL")
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.StringVar(&configPath, "config", "", "YAML configuration path")

	_ = fs.Parse(args)

	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err == nil {
			if port == 8080 && cfg.Server.Port != 0 {
				port = cfg.Server.Port
			}
			if forwardTo == "" && cfg.Upstream.DefaultTargetURL != "" {
				forwardTo = cfg.Upstream.DefaultTargetURL
			}
		}
	}

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	srv := proxy.NewServer(proxy.ServerConfig{
		Addr:       addr,
		ForwardURL: forwardTo,
		Store:      st,
		Version:    version,
	})

	fmt.Printf("webhook-replay-proxy %s starting\n", version)
	fmt.Printf("• Listening on: http://%s\n", addr)
	if forwardTo != "" {
		fmt.Printf("• Forwarding to: %s\n", forwardTo)
	} else {
		fmt.Println("• Forwarding: disabled (recording only)")
	}
	fmt.Printf("• Storage: SQLite (%s)\n\n", dbPath)

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != proxy.ErrServerClosed() {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-stopChan
	fmt.Println("\nShutting down webhook-replay-proxy gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	var (
		dbPath string
		limit  int
	)
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.IntVar(&limit, "limit", 50, "Limit number of records")
	_ = fs.Parse(args)

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	events, err := st.ListEvents(limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list events: %v\n", err)
		os.Exit(1)
	}

	if len(events) == 0 {
		fmt.Println("No webhooks captured yet.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTIMESTAMP\tPROVIDER\tMETHOD\tPATH\tSTATUS\tLATENCY")
	for _, e := range events {
		statusStr := "-"
		if e.ForwardStatus > 0 {
			statusStr = strconv.Itoa(e.ForwardStatus)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%dms\n",
			e.ID,
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Provider,
			e.Method,
			e.Path,
			statusStr,
			e.LatencyMs,
		)
	}
	_ = w.Flush()
}

func runInspect(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "Error: <event_id> argument is required for inspect")
		fmt.Fprintln(os.Stderr, "Usage: webhook-replay-proxy inspect <event_id> [flags]")
		os.Exit(1)
	}

	eventID := args[0]
	args = args[1:]

	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	var (
		dbPath  string
		rawBody bool
	)
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.BoolVar(&rawBody, "raw", false, "Print raw unformatted payload body")
	_ = fs.Parse(args)

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	event, err := st.GetEvent(eventID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Event ID:       %s\n", event.ID)
	fmt.Printf("Recorded At:    %s\n", event.Timestamp.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Provider:       %s\n", event.Provider)
	fmt.Printf("Request:        %s %s\n", event.Method, event.Path)
	if event.Query != "" {
		fmt.Printf("Query:          %s\n", event.Query)
	}
	fmt.Printf("Target URL:     %s\n", event.TargetURL)
	fmt.Printf("Forward Status: %d (%dms)\n", event.ForwardStatus, event.LatencyMs)

	fmt.Println("\n--- HTTP Headers ---")
	for k, vv := range event.Headers {
		for _, v := range vv {
			fmt.Printf("%s: %s\n", k, v)
		}
	}

	fmt.Println("\n--- Payload Body ---")
	if rawBody {
		fmt.Println(string(event.Body))
	} else {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, event.Body, "", "  "); err == nil {
			fmt.Println(pretty.String())
		} else {
			fmt.Println(string(event.Body))
		}
	}

	if event.ResponseBody != "" {
		fmt.Println("\n--- Target Response Body ---")
		fmt.Println(event.ResponseBody)
	}

	replays, _ := st.GetReplaysForEvent(eventID)
	if len(replays) > 0 {
		fmt.Printf("\n--- Replay History (%d) ---\n", len(replays))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIMESTAMP\tTARGET\tSTATUS\tLATENCY\tRESULT")
		for _, r := range replays {
			res := "OK"
			if !r.Success {
				res = "FAILED"
				if r.Error != "" {
					res += " (" + r.Error + ")"
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%dms\t%s\n",
				r.Timestamp.Format("15:04:05"),
				r.TargetURL,
				r.Status,
				r.LatencyMs,
				res,
			)
		}
		_ = w.Flush()
	}
}

func runReplay(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "Error: <event_id> argument is required for replay")
		fmt.Fprintln(os.Stderr, "Usage: webhook-replay-proxy replay <event_id> [flags]")
		os.Exit(1)
	}

	eventID := args[0]
	args = args[1:]

	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	var (
		targetURL   string
		provider    string
		secret      string
		concurrency int
		delay       time.Duration
		showDiff    bool
		dbPath      string
		mutations   arrayFlags
	)

	fs.StringVar(&targetURL, "target", "", "Target URL override")
	fs.StringVar(&provider, "provider", "auto", "Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fs.StringVar(&secret, "secret", "", "Webhook HMAC secret to re-sign payload")
	fs.IntVar(&concurrency, "concurrency", 1, "Number of concurrent replayed requests")
	fs.DurationVar(&delay, "delay", 0, "Delay between paced sequential requests")
	fs.BoolVar(&showDiff, "diff", false, "Display visual diff of mutated JSON payload before replaying")
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.Var(&mutations, "set", "Mutate JSON field (key.path=value)")

	_ = fs.Parse(args)

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	engine := proxy.NewReplayEngine(st)
	fmt.Printf("Replaying event: %s\n", eventID)
	if len(mutations) > 0 {
		fmt.Printf("• Mutations (%d): %s\n", len(mutations), strings.Join(mutations, ", "))
	}
	if secret != "" {
		fmt.Printf("• Re-signing with provider: %s\n", provider)
	}
	if delay > 0 {
		fmt.Printf("• Paced delay: %v\n", delay)
	}
	fmt.Printf("• Concurrency: %d request(s)\n\n", concurrency)

	start := time.Now()
	results, err := engine.Execute(context.Background(), proxy.ReplayOptions{
		EventID:     eventID,
		TargetURL:   targetURL,
		Provider:    provider,
		Secret:      secret,
		Mutations:   mutations,
		Concurrency: concurrency,
		Delay:       delay,
		ShowDiff:    showDiff,
	})
	totalTime := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Replay failed: %v\n", err)
		os.Exit(1)
	}

	successCount := 0
	for _, res := range results {
		if res.Error != nil {
			fmt.Printf("[%d] FAILED: %v (latency: %dms)\n", res.Index+1, res.Error, res.LatencyMs)
		} else {
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				successCount++
			}
			fmt.Printf("[%d] HTTP %d | Latency: %dms\n", res.Index+1, res.StatusCode, res.LatencyMs)
		}
	}

	fmt.Printf("\nFinished: %d/%d successful in %dms total\n", successCount, concurrency, totalTime)
}

func runCurl(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "Error: <event_id> argument is required for curl generator")
		fmt.Fprintln(os.Stderr, "Usage: webhook-replay-proxy curl <event_id> [flags]")
		os.Exit(1)
	}

	eventID := args[0]
	args = args[1:]

	fs := flag.NewFlagSet("curl", flag.ExitOnError)
	var (
		targetURL string
		provider  string
		secret    string
		dbPath    string
		mutations arrayFlags
	)

	fs.StringVar(&targetURL, "target", "", "Target URL override")
	fs.StringVar(&provider, "provider", "auto", "Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fs.StringVar(&secret, "secret", "", "Webhook HMAC secret to re-sign payload")
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.Var(&mutations, "set", "Mutate JSON field (key.path=value)")

	_ = fs.Parse(args)

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	engine := proxy.NewReplayEngine(st)
	cmdStr, err := engine.GenerateCurl(proxy.ReplayOptions{
		EventID:   eventID,
		TargetURL: targetURL,
		Provider:  provider,
		Secret:    secret,
		Mutations: mutations,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate curl command: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(cmdStr)
}
