package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"net_rules_aggregator/internal/aggregator"
	"net_rules_aggregator/internal/cache"
	"net_rules_aggregator/internal/formatter"
	"net_rules_aggregator/internal/input"
	"net_rules_aggregator/internal/resolver"
)

var (
	flagInput       string
	flagFormat      string
	flagOutput      string
	flagCacheFile   string
	flagSkipResolve bool
	flagIPVersion   string
	flagConcurrency int
	flagTimeout     time.Duration
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "net_rules_aggregator",
	Short: "Generate subnet lists for Keenetic route and AmneziaVPN split tunneling",
	Long: `net_rules_aggregator resolves ASNs, domains, and IP addresses to subnet lists
and formats them for import into Keenetic (static routes) or AmneziaVPN (split tunneling).

Pipeline:
  1. Read input YAML (--input)
  2. Resolve domains → IP → ASN via DNS + RIPEstat API
  3. Fetch all prefixes for collected ASNs from RIPEstat
  4. Aggregate/summarise CIDRs
  5. Save intermediate cache (--cache-file)
  6. Format and write output (--format, --output)

Use --skip-resolve to skip steps 2-3 and load from existing cache.`,
	RunE: run,
}

func init() {
	rootCmd.Flags().StringVarP(&flagInput, "input", "i", "input.yaml", "Path to input YAML file")
	rootCmd.Flags().StringVarP(&flagFormat, "format", "f", "cidr", "Output format: keenetic, amnezia, cidr, yaml")
	rootCmd.Flags().StringVarP(&flagOutput, "output", "o", "", "Output file path (default: stdout)")
	rootCmd.Flags().StringVar(&flagCacheFile, "cache-file", "cache.yaml", "Intermediate cache YAML file")
	rootCmd.Flags().BoolVar(&flagSkipResolve, "skip-resolve", false, "Skip resolution, load from existing cache file")
	rootCmd.Flags().StringVar(&flagIPVersion, "ip-version", "4", "IP version to include: 4, 6, both")
	rootCmd.Flags().IntVar(&flagConcurrency, "concurrency", 5, "Parallel requests to RIPE API")
	rootCmd.Flags().DurationVar(&flagTimeout, "timeout", 30*time.Second, "HTTP request timeout")
}

func run(cmd *cobra.Command, _ []string) error {
	// Validate --format
	format := strings.ToLower(strings.TrimSpace(flagFormat))
	switch format {
	case "keenetic", "amnezia", "cidr", "yaml":
	default:
		return fmt.Errorf("unknown format %q: must be one of keenetic, amnezia, cidr, yaml", flagFormat)
	}

	// Validate --ip-version
	ipVersion, err := parseIPVersion(flagIPVersion)
	if err != nil {
		return err
	}

	// Prepare output writer.
	var out io.Writer = os.Stdout
	if flagOutput != "" {
		f, err := os.Create(flagOutput)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	warn := func(msg string) {
		fmt.Fprintln(os.Stderr, "[WARN]", msg)
	}

	var entries []resolver.PrefixEntry

	if flagSkipResolve {
		// Load from cache directly.
		fmt.Fprintf(os.Stderr, "[INFO] Loading from cache: %s\n", flagCacheFile)
		entries, err = cache.Load(flagCacheFile)
		if err != nil {
			return fmt.Errorf("loading cache: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[INFO] Loaded %d prefixes from cache\n", len(entries))
	} else {
		// Full resolution pipeline.
		fmt.Fprintf(os.Stderr, "[INFO] Loading input: %s\n", flagInput)
		validated, err := input.Load(flagInput)
		if err != nil {
			return fmt.Errorf("loading input: %w", err)
		}

		fmt.Fprintf(os.Stderr, "[INFO] Input: %d ASNs, %d domains, %d IPs, %d prefixes\n",
			len(validated.ASNs), len(validated.Domains), len(validated.RawIPs), len(validated.Prefixes))

		opts := resolver.Options{
			IPVersion:   ipVersion,
			Concurrency: flagConcurrency,
			Timeout:     flagTimeout,
		}

		fmt.Fprintln(os.Stderr, "[INFO] Resolving...")
		entries, err = resolver.ResolveAll(
			cmd.Context(),
			validated.ASNs,
			validated.Domains,
			validated.RawIPs,
			validated.Prefixes,
			opts,
			warn,
		)
		if err != nil {
			return fmt.Errorf("resolving: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[INFO] Resolved %d raw prefixes\n", len(entries))

		// Aggregate.
		fmt.Fprintln(os.Stderr, "[INFO] Aggregating...")
		entries = aggregator.Aggregate(entries)
		fmt.Fprintf(os.Stderr, "[INFO] Aggregated to %d prefixes\n", len(entries))

		// Save cache.
		allASNs := collectASNs(entries)
		if err := cache.Save(flagCacheFile, entries, allASNs); err != nil {
			warn(fmt.Sprintf("saving cache: %v", err))
		} else {
			fmt.Fprintf(os.Stderr, "[INFO] Cache saved: %s\n", flagCacheFile)
		}
	}

	// Format output.
	fmt.Fprintf(os.Stderr, "[INFO] Formatting as %q (%d prefixes)\n", format, len(entries))
	switch format {
	case "keenetic":
		err = formatter.Keenetic(out, entries)
	case "amnezia":
		err = formatter.Amnezia(out, entries)
	case "cidr":
		err = formatter.CIDR(out, entries)
	case "yaml":
		err = formatter.YAML(out, entries)
	}
	if err != nil {
		return fmt.Errorf("formatting output: %w", err)
	}

	if flagOutput != "" {
		fmt.Fprintf(os.Stderr, "[INFO] Output written to: %s\n", flagOutput)
	}
	return nil
}

// parseIPVersion converts the --ip-version flag string to an int.
// Returns 4, 6, or 0 (both).
func parseIPVersion(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "4":
		return 4, nil
	case "6":
		return 6, nil
	case "both", "0", "all":
		return 0, nil
	default:
		return 0, fmt.Errorf("invalid --ip-version %q: must be 4, 6, or both", s)
	}
}

// collectASNs returns a deduplicated list of ASNs from prefix entries.
func collectASNs(entries []resolver.PrefixEntry) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, e := range entries {
		if e.ASN != "" {
			if _, ok := seen[e.ASN]; !ok {
				seen[e.ASN] = struct{}{}
				out = append(out, e.ASN)
			}
		}
	}
	return out
}
