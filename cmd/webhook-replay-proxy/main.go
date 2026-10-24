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
	version = "0.2.0"
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
	fmt.Println("  export <event_id>  Export recorded webhook to a standalone JSON fixture file")
	fmt.Println("  replay <event_id>  Replay a captured webhook with mutations and HMAC re-signing")
	fmt.Println("  curl <event_id>    Generate ready-to-run curl command with fresh HMAC signature")
	fmt.Println("  version            Show version information")
	fmt.Println()
	fmt.Println("Flags for 'listen':")
	fmt.Println("  --port, -port <int>        Listen port (default: 8080)")
	fmt.Println("  --forward-to <url>         Forward incoming webhooks to target URL")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println("  --config <path>            Path to YAML configuration file")
	fmt.Println("  --ui                       Enable embedded web dashboard at /_dashboard")
	fmt.Println()
	fmt.Println("Flags for 'list':")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println("  --limit <int>              Maximum events to list (default: 50)")
	fmt.Println()
	fmt.Println("Flags for 'inspect':")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println("  --raw                      Print raw unformatted payload body")
	fmt.Println()
	fmt.Println("Flags for 'export':")
	fmt.Println("  --output, -o <path>        Output file path (default: stdout)")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println()
	fmt.Println("Flags for 'replay':")
	fmt.Println("  --file <path>              Replay directly from a fixture JSON file without SQLite")
	fmt.Println("  --target <url>             Override destination target URL")
	fmt.Println("  --provider <name>          Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fmt.Println("  --secret <string>          Webhook HMAC secret to re-sign payload with fresh timestamp")
	fmt.Println("  --set <key.path=value>     Mutate JSON field (can be specified multiple times)")
	fmt.Println("  --concurrency <int>        Number of concurrent requests fired simultaneously (default: 1)")
	fmt.Println("  --delay <duration>         Delay between spaced requests (e.g. 100ms, 1s)")
	fmt.Println("  --diff                     Display visual diff of mutated JSON payload before replaying")
	fmt.Println("  --corrupt-sig              Deliberately corrupt HMAC signature to verify 401 rejection")
	fmt.Println("  --corrupt-json             Deliberately malform payload JSON to test parser error handling")
	fmt.Println("  --drop-header <name>       Strip specified header before dispatching (can be repeated)")
	fmt.Println("  --time-offset <duration>   Offset signing timestamp from current time (e.g. -10m, +1h)")
	fmt.Println("  --fixed-timestamp <unix>   Use an exact fixed unix epoch timestamp for signing")
	fmt.Println("  --assert-status <pattern>  Assert response status distribution (e.g. '200:1,409:9' or '200')")
	fmt.Println("  --max-latency <duration>   Fail assertion if any request latency exceeds threshold")
	fmt.Println("  --db <path>                Path to SQLite database (default: ./webhooks.db)")
	fmt.Println()
	fmt.Println("Flags for 'curl':")
	fmt.Println("  --file <path>              Generate curl directly from a fixture JSON file")
	fmt.Println("  --target <url>             Override destination target URL")
	fmt.Println("  --provider <name>          Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fmt.Println("  --secret <string>          Webhook HMAC secret to re-sign payload with fresh timestamp")
	fmt.Println("  --set <key.path=value>     Mutate JSON field (can be specified multiple times)")
	fmt.Println("  --corrupt-sig              Deliberately corrupt HMAC signature")
	fmt.Println("  --corrupt-json             Deliberately malform payload JSON")
	fmt.Println("  --drop-header <name>       Strip specified header from generated curl command")
	fmt.Println("  --time-offset <duration>   Offset signing timestamp from current time")
	fmt.Println("  --fixed-timestamp <unix>   Use an exact fixed unix epoch timestamp for signing")
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
	case "export":
		runExport(args)
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
		enableUI   bool
	)

	fs.IntVar(&port, "port", 8080, "Port to listen on")
	fs.StringVar(&forwardTo, "forward-to", "", "Forward incoming webhooks to target URL")
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.StringVar(&configPath, "config", "", "Configuration file path")
	fs.BoolVar(&enableUI, "ui", false, "Enable embedded web dashboard at /_dashboard")

	_ = fs.Parse(args)

	portVal := port
	fwdVal := forwardTo
	dbVal := dbPath

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load config file: %v\n", err)
		} else {
			if portVal == 8080 && loaded.Server.Port != 0 {
				portVal = loaded.Server.Port
			}
			if fwdVal == "" && loaded.Upstream.DefaultTargetURL != "" {
				fwdVal = loaded.Upstream.DefaultTargetURL
			}
		}
	}

	st, err := store.Open(dbVal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbVal, err)
		os.Exit(1)
	}
	defer st.Close()

	addr := fmt.Sprintf(":%d", portVal)
	srv := proxy.NewServer(proxy.ServerConfig{
		Addr:       addr,
		ForwardURL: fwdVal,
		Store:      st,
		Version:    version,
		EnableUI:   enableUI,
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("⚡ Webhook Replay Proxy listening on %s\n", addr)
		if fwdVal != "" {
			fmt.Printf("  • Forwarding to: %s\n", fwdVal)
		}
		fmt.Printf("  • Flight recorder: %s (pure-Go SQLite WAL)\n", dbVal)
		fmt.Printf("  • Live Dashboard: http://localhost:%d/_dashboard\n", portVal)
		fmt.Println("Ready to capture incoming webhooks. Press Ctrl+C to stop.")

		if err := srv.Start(); err != nil && err != proxy.ErrServerClosed() {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-stop
	fmt.Println("\nShutting down proxy gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
	}
	fmt.Println("Server stopped.")
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	var (
		dbPath string
		limit  int
	)

	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.IntVar(&limit, "limit", 50, "Maximum events to list")
	_ = fs.Parse(args)

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	events, err := st.ListEvents(limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query events: %v\n", err)
		os.Exit(1)
	}

	if len(events) == 0 {
		fmt.Println("No webhooks recorded yet.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EVENT ID\tTIMESTAMP\tPROVIDER\tMETHOD\tPATH\tFORWARDED\tLATENCY")
	for _, e := range events {
		forwardStr := strconv.Itoa(e.ForwardStatus)
		if e.ForwardStatus == 0 {
			forwardStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%dms\n",
			e.ID,
			e.Timestamp.Format("15:04:05"),
			e.Provider,
			e.Method,
			e.Path,
			forwardStr,
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
		dbPath string
		raw    bool
	)
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.BoolVar(&raw, "raw", false, "Print raw unformatted payload body")
	_ = fs.Parse(args)

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	event, err := st.GetEvent(eventID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Event %s not found: %v\n", eventID, err)
		os.Exit(1)
	}

	fmt.Printf("Event ID:       %s\n", event.ID)
	fmt.Printf("Timestamp:      %s\n", event.Timestamp.Format(time.RFC3339))
	fmt.Printf("Provider:       %s\n", event.Provider)
	fmt.Printf("Method/Path:    %s %s\n", event.Method, event.Path)
	if event.Query != "" {
		fmt.Printf("Query:          %s\n", event.Query)
	}
	if event.TargetURL != "" {
		fmt.Printf("Target URL:     %s\n", event.TargetURL)
		fmt.Printf("Forward Status: %d (%dms)\n", event.ForwardStatus, event.LatencyMs)
	}

	fmt.Println("\n--- Request Headers ---")
	for k, vv := range event.Headers {
		for _, v := range vv {
			fmt.Printf("%s: %s\n", k, v)
		}
	}

	fmt.Println("\n--- Request Body ---")
	if raw {
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

func runExport(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "Error: <event_id> argument is required for export")
		fmt.Fprintln(os.Stderr, "Usage: webhook-replay-proxy export <event_id> [flags]")
		os.Exit(1)
	}

	eventID := args[0]
	args = args[1:]

	fs := flag.NewFlagSet("export", flag.ExitOnError)
	var (
		outputPath string
		dbPath     string
	)

	fs.StringVar(&outputPath, "output", "", "Output file path (default: stdout)")
	fs.StringVar(&outputPath, "o", "", "Output file path (shorthand)")
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	_ = fs.Parse(args)

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer st.Close()

	event, err := st.GetEvent(eventID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Event %s not found: %v\n", eventID, err)
		os.Exit(1)
	}

	data, err := proxy.ExportFixture(event)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to export fixture: %v\n", err)
		os.Exit(1)
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write fixture file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Exported event %s to %s\n", eventID, outputPath)
	} else {
		fmt.Println(string(data))
	}
}

func runReplay(args []string) {
	var eventID string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		eventID = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	var (
		fixturePath    string
		targetURL      string
		provider       string
		secret         string
		concurrency    int
		delay          time.Duration
		showDiff       bool
		corruptSig     bool
		corruptJSON    bool
		timeOffset     time.Duration
		fixedTimestamp int64
		assertStatus   string
		maxLatency     time.Duration
		dbPath         string
		mutations      arrayFlags
		dropHeaders    arrayFlags
	)

	fs.StringVar(&fixturePath, "file", "", "Path to fixture JSON file to replay")
	fs.StringVar(&targetURL, "target", "", "Target URL override")
	fs.StringVar(&provider, "provider", "auto", "Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fs.StringVar(&secret, "secret", "", "Webhook HMAC secret to re-sign payload")
	fs.IntVar(&concurrency, "concurrency", 1, "Number of concurrent replayed requests")
	fs.DurationVar(&delay, "delay", 0, "Delay between paced sequential requests")
	fs.BoolVar(&showDiff, "diff", false, "Display visual diff of mutated JSON payload before replaying")
	fs.BoolVar(&corruptSig, "corrupt-sig", false, "Deliberately corrupt HMAC signature to verify 401 rejection")
	fs.BoolVar(&corruptJSON, "corrupt-json", false, "Deliberately malform payload JSON to test parser error handling")
	fs.DurationVar(&timeOffset, "time-offset", 0, "Offset signing timestamp from current time (e.g. -10m, +1h)")
	fs.Int64Var(&fixedTimestamp, "fixed-timestamp", 0, "Use an exact fixed unix epoch timestamp for signing")
	fs.StringVar(&assertStatus, "assert-status", "", "Assert response status distribution (e.g. '200:1,409:9' or '200')")
	fs.DurationVar(&maxLatency, "max-latency", 0, "Fail assertion if any request latency exceeds threshold")
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.Var(&mutations, "set", "Mutate JSON field (key.path=value)")
	fs.Var(&dropHeaders, "drop-header", "Strip specified header before dispatching")

	_ = fs.Parse(args)

	if eventID == "" && fixturePath == "" {
		fmt.Fprintln(os.Stderr, "Error: <event_id> or --file <path> is required for replay")
		fmt.Fprintln(os.Stderr, "Usage: webhook-replay-proxy replay [<event_id> | --file <path>] [flags]")
		os.Exit(1)
	}

	var st *store.Store
	if fixturePath == "" {
		var err error
		st, err = store.Open(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
			os.Exit(1)
		}
		defer st.Close()
	}

	engine := proxy.NewReplayEngine(st)
	if fixturePath != "" {
		fmt.Printf("Replaying from fixture: %s\n", fixturePath)
	} else {
		fmt.Printf("Replaying event: %s\n", eventID)
	}

	if len(mutations) > 0 {
		fmt.Printf("• Mutations (%d): %s\n", len(mutations), strings.Join(mutations, ", "))
	}
	if secret != "" {
		fmt.Printf("• Re-signing with provider: %s\n", provider)
	}
	if corruptSig {
		fmt.Println("• Chaos Mode: Corrupting HMAC signature")
	}
	if corruptJSON {
		fmt.Println("• Chaos Mode: Malforming JSON body syntax")
	}
	if len(dropHeaders) > 0 {
		fmt.Printf("• Dropping headers: %s\n", strings.Join(dropHeaders, ", "))
	}
	if timeOffset != 0 {
		fmt.Printf("• Time-travel offset: %v\n", timeOffset)
	}
	if fixedTimestamp > 0 {
		fmt.Printf("• Fixed timestamp: %d\n", fixedTimestamp)
	}
	if delay > 0 {
		fmt.Printf("• Paced delay: %v\n", delay)
	}
	fmt.Printf("• Concurrency: %d request(s)\n\n", concurrency)

	start := time.Now()
	results, err := engine.Execute(context.Background(), proxy.ReplayOptions{
		EventID:        eventID,
		FixturePath:    fixturePath,
		TargetURL:      targetURL,
		Provider:       provider,
		Secret:         secret,
		Mutations:      mutations,
		Concurrency:    concurrency,
		Delay:          delay,
		ShowDiff:       showDiff,
		CorruptSig:     corruptSig,
		CorruptJSON:    corruptJSON,
		DropHeaders:    dropHeaders,
		TimeOffset:     timeOffset,
		FixedTimestamp: fixedTimestamp,
		AssertStatus:   assertStatus,
		MaxLatency:     maxLatency,
	})
	totalTime := time.Since(start).Milliseconds()

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

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nReplay failed assertion or error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nFinished: %d/%d successful in %dms total\n", successCount, concurrency, totalTime)
}

func runCurl(args []string) {
	var eventID string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		eventID = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("curl", flag.ExitOnError)
	var (
		fixturePath    string
		targetURL      string
		provider       string
		secret         string
		corruptSig     bool
		corruptJSON    bool
		timeOffset     time.Duration
		fixedTimestamp int64
		dbPath         string
		mutations      arrayFlags
		dropHeaders    arrayFlags
	)

	fs.StringVar(&fixturePath, "file", "", "Path to fixture JSON file")
	fs.StringVar(&targetURL, "target", "", "Target URL override")
	fs.StringVar(&provider, "provider", "auto", "Provider signing scheme (stripe, github, slack, shopify, standard, auto)")
	fs.StringVar(&secret, "secret", "", "Webhook HMAC secret to re-sign payload")
	fs.BoolVar(&corruptSig, "corrupt-sig", false, "Deliberately corrupt HMAC signature")
	fs.BoolVar(&corruptJSON, "corrupt-json", false, "Deliberately malform payload JSON")
	fs.DurationVar(&timeOffset, "time-offset", 0, "Offset signing timestamp from current time")
	fs.Int64Var(&fixedTimestamp, "fixed-timestamp", 0, "Use an exact fixed unix epoch timestamp")
	fs.StringVar(&dbPath, "db", "./webhooks.db", "SQLite database path")
	fs.Var(&mutations, "set", "Mutate JSON field (key.path=value)")
	fs.Var(&dropHeaders, "drop-header", "Strip specified header")

	_ = fs.Parse(args)

	if eventID == "" && fixturePath == "" {
		fmt.Fprintln(os.Stderr, "Error: <event_id> or --file <path> is required for curl generator")
		fmt.Fprintln(os.Stderr, "Usage: webhook-replay-proxy curl [<event_id> | --file <path>] [flags]")
		os.Exit(1)
	}

	var st *store.Store
	if fixturePath == "" {
		var err error
		st, err = store.Open(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open SQLite database at %s: %v\n", dbPath, err)
			os.Exit(1)
		}
		defer st.Close()
	}

	engine := proxy.NewReplayEngine(st)
	cmdStr, err := engine.GenerateCurl(proxy.ReplayOptions{
		EventID:        eventID,
		FixturePath:    fixturePath,
		TargetURL:      targetURL,
		Provider:       provider,
		Secret:         secret,
		Mutations:      mutations,
		CorruptSig:     corruptSig,
		CorruptJSON:    corruptJSON,
		DropHeaders:    dropHeaders,
		TimeOffset:     timeOffset,
		FixedTimestamp: fixedTimestamp,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate curl command: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(cmdStr)
}
