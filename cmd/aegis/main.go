// Command aegis is the CLI and server entrypoint.
//
//	aegis init     --repo DIR
//	aegis backup   --repo DIR SOURCE
//	aegis list     --repo DIR
//	aegis restore  --repo DIR [--id ID | --latest-clean] DEST
//	aegis verify   --repo DIR [--deep]
//	aegis serve    --repo DIR --source DIR [--addr :8080]
//
// The passphrase is read from the AEGIS_PASSPHRASE environment variable (in
// Kubernetes it is mounted from a Secret).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/sriharshini/aegis/internal/repo"
	"github.com/sriharshini/aegis/internal/server"
	"github.com/sriharshini/aegis/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: aegis <init|backup|list|restore|verify|serve> [flags]`)
}

func run(ctx context.Context, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	repoDir := fs.String("repo", envOr("AEGIS_REPO", "./aegis-repo"), "repository directory")
	retention := fs.Duration("retention", 30*24*time.Hour, "WORM retention period for new objects")
	id := fs.String("id", "", "snapshot id (restore)")
	latestClean := fs.Bool("latest-clean", false, "restore the most recent snapshot not flagged as ransomware (restore)")
	deep := fs.Bool("deep", false, "decrypt and re-hash every chunk (verify)")
	addr := fs.String("addr", ":8080", "listen address (serve)")
	source := fs.String("source", "", "directory to protect (serve)")
	asJSON := fs.Bool("json", false, "JSON output")
	fs.Parse(args)

	pass := os.Getenv("AEGIS_PASSPHRASE")
	if pass == "" {
		return errors.New("AEGIS_PASSPHRASE is not set")
	}
	st, err := store.NewLocalWORM(*repoDir, *retention)
	if err != nil {
		return err
	}

	if cmd == "init" {
		if _, err := repo.Init(ctx, st, pass); err != nil {
			return err
		}
		fmt.Println("initialised repository at", *repoDir)
		return nil
	}
	r, err := repo.Open(ctx, st, pass)
	if err != nil {
		return err
	}

	switch cmd {
	case "backup":
		if fs.NArg() != 1 {
			return errors.New("usage: aegis backup --repo DIR SOURCE")
		}
		sn, err := r.Backup(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(sn.Stats, sn.Verdict)
		}
		fmt.Printf("snapshot %s  files=%d  size=%s  uploaded=%s  chunks=%d (new %d)  %dms\n",
			sn.ID, sn.Stats.Files, human(sn.Stats.Bytes), human(sn.Stats.NewBytes),
			sn.Stats.Chunks, sn.Stats.NewChunks, sn.Stats.DurationMS)
		printVerdict(sn)
		return nil

	case "list":
		all, err := r.Snapshots(ctx)
		if errors.Is(err, repo.ErrNoSnapshots) {
			fmt.Println("no snapshots")
			return nil
		}
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(all)
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tTIME\tFILES\tSIZE\tUPLOADED\tSCORE\tSTATUS")
		for _, s := range all {
			status := "clean"
			if s.Verdict.Suspicious {
				status = "SUSPICIOUS"
			}
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%.2f\t%s\n", s.ID, s.Time.Format(time.RFC3339),
				s.Stats.Files, human(s.Stats.Bytes), human(s.Stats.NewBytes), s.Verdict.Score, status)
		}
		return tw.Flush()

	case "restore":
		if fs.NArg() != 1 {
			return errors.New("usage: aegis restore --repo DIR [--id ID | --latest-clean] DEST")
		}
		target := *id
		if *latestClean {
			sn, err := r.LatestClean(ctx)
			if err != nil {
				return err
			}
			target = sn.ID
			fmt.Println("latest clean recovery point:", sn.ID, sn.Time.Format(time.RFC3339))
		}
		if target == "" {
			return errors.New("pass --id or --latest-clean")
		}
		if err := r.Restore(ctx, target, fs.Arg(0)); err != nil {
			return err
		}
		fmt.Println("restored", target, "to", fs.Arg(0), "(every file SHA-256 verified)")
		return nil

	case "verify":
		rep, err := r.Verify(ctx, *deep)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(rep)
		}
		fmt.Printf("snapshots=%d chunks=%d deep=%v\n", rep.Snapshots, rep.ChunksChecked, *deep)
		for _, p := range rep.Problems {
			fmt.Println("  PROBLEM:", p)
		}
		if len(rep.Problems) > 0 {
			return fmt.Errorf("%d integrity problems", len(rep.Problems))
		}
		fmt.Println("OK: hash chain intact, all chunks present")
		return nil

	case "serve":
		if *source == "" {
			return errors.New("--source is required")
		}
		log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
		srv := &http.Server{Addr: *addr, Handler: server.New(r, *source, log).Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			srv.Shutdown(shutdownCtx) // graceful: let in-flight backup finish
		}()
		log.Info("aegis listening", "addr", *addr, "source", *source)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	usage()
	return fmt.Errorf("unknown command %q", cmd)
}

func printVerdict(sn *repo.Snapshot) {
	v := sn.Verdict
	if !v.Suspicious {
		fmt.Printf("anomaly score %.2f - looks clean\n", v.Score)
		return
	}
	fmt.Printf("\n!!! RANSOMWARE INDICATORS - anomaly score %.2f !!!\n", v.Score)
	for _, r := range v.Reasons {
		fmt.Println("   -", r)
	}
	fmt.Println("   This snapshot will NOT be used as a recovery point. Use: aegis restore --latest-clean DEST")
}

func printJSON(v ...any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	for _, x := range v {
		if err := enc.Encode(x); err != nil {
			return err
		}
	}
	return nil
}

func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
