package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"

	"github.com/KashifKhn/remove-comments/cli/internal/config"
	"github.com/KashifKhn/remove-comments/cli/internal/diff"
	"github.com/KashifKhn/remove-comments/cli/internal/generated"
	"github.com/KashifKhn/remove-comments/cli/internal/languages"
	"github.com/KashifKhn/remove-comments/cli/internal/output"
	"github.com/KashifKhn/remove-comments/cli/internal/parser"
	"github.com/KashifKhn/remove-comments/cli/internal/remover"
	"github.com/KashifKhn/remove-comments/cli/internal/walker"
)

var rootCmd = &cobra.Command{
	Use:   "remove-comments [path]",
	Short: "Remove all comments from source files in a directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  run,
}

var (
	flagWrite           bool
	flagQuiet           bool
	flagDiff            bool
	flagLang            string
	flagJobs            int
	flagMaxFileSize     int64
	flagExclude         []string
	flagInclude         []string
	flagNoSkipGenerated bool
	flagNoConfig        bool
	flagConfigPath      string
	flagListLangs       bool
)

func Execute(version string) {
	rootCmd.Version = version
	registerUpgradeCmd(version)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&flagWrite, "write", "w", false, "Write changes to disk (default is dry-run)")
	rootCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "Print only the final summary line")
	rootCmd.Flags().BoolVarP(&flagDiff, "diff", "d", false, "Show unified diff for each changed file")
	rootCmd.Flags().StringVar(&flagLang, "lang", "", "Only process files of these languages, comma-separated (e.g. go,java)")
	rootCmd.Flags().IntVarP(&flagJobs, "jobs", "j", 0, "Number of parallel workers (default: NumCPU)")
	rootCmd.Flags().Int64Var(&flagMaxFileSize, "max-file-size", 10*1024*1024, "Skip files larger than this size in bytes")
	rootCmd.Flags().StringArrayVarP(&flagExclude, "exclude", "e", nil, "Glob patterns to exclude (e.g. '*.g.dart', 'vendor/**')")
	rootCmd.Flags().StringArrayVar(&flagInclude, "include", nil, "Glob patterns to include; when set, only matching files are processed (e.g. 'src/**')")
	rootCmd.Flags().BoolVar(&flagNoSkipGenerated, "no-skip-generated", false, "Do not skip generated files (*.g.dart, *.pb.go, 'DO NOT EDIT' markers)")
	rootCmd.Flags().BoolVar(&flagNoConfig, "no-config", false, "Do not load .remove-comments.yaml config file")
	rootCmd.Flags().StringVar(&flagConfigPath, "config", "", "Path to a config file (default: search for .remove-comments.yaml)")
	rootCmd.Flags().BoolVar(&flagListLangs, "list-langs", false, "List all supported languages and exit")
}

func run(cmd *cobra.Command, args []string) error {
	if flagListLangs {
		for _, name := range languages.Names() {
			fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		return nil
	}

	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	if _, err := os.Stat(root); err != nil {
		return err
	}

	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	langNames := parseLangs(flagLang, cfg.Langs)
	filter := languages.NewLangFilter(langNames)
	if err := filter.Validate(); err != nil {
		return err
	}

	opts := walker.Options{
		Langs:       filter.Names(),
		MaxFileSize: resolveMaxFileSize(cmd, cfg.MaxFileSize),
		Exclude:     mergeStrings(cfg.Exclude, flagExclude),
		Include:     mergeStrings(cfg.Include, flagInclude),
	}

	skipGenerated := !flagNoSkipGenerated
	if cfg.SkipGenerated != nil {
		skipGenerated = *cfg.SkipGenerated && !flagNoSkipGenerated
	}

	jobs := flagJobs
	if jobs <= 0 {
		jobs = runtime.NumCPU()
	}

	entries, walkErrs := walker.Walk(root, opts)
	if len(walkErrs) > 0 {
		for _, e := range walkErrs {
			fmt.Fprintf(os.Stderr, "walk error: %v\n", e)
		}
	}

	printer := output.New(os.Stdout, flagQuiet, flagWrite, flagDiff)

	var (
		mu      sync.Mutex
		changed int32
		errors  int32
		skipped int32
		total   int32
	)

	work := make(chan walker.FileEntry, jobs*2)
	var wg sync.WaitGroup

	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range work {
				atomic.AddInt32(&total, 1)

				src, err := os.ReadFile(entry.Path)
				if err != nil {
					atomic.AddInt32(&errors, 1)
					mu.Lock()
					printer.Error(entry.Path, err)
					mu.Unlock()
					continue
				}

				if skipGenerated && generated.IsGenerated(entry.Path, src) {
					atomic.AddInt32(&skipped, 1)
					mu.Lock()
					printer.Skipped(entry.Path, "generated")
					mu.Unlock()
					continue
				}

				ranges, err := parser.Parse(src, entry.Lang)
				if err != nil {
					atomic.AddInt32(&errors, 1)
					mu.Lock()
					printer.Error(entry.Path, err)
					mu.Unlock()
					continue
				}

				after := remover.Remove(src, ranges)
				result := diff.Compute(entry.Path, src, after)

				if result.Changed {
					if flagWrite {
						info, statErr := os.Stat(entry.Path)
						if statErr != nil {
							atomic.AddInt32(&errors, 1)
							mu.Lock()
							printer.Error(entry.Path, statErr)
							mu.Unlock()
							continue
						}
						if info.Mode()&0o200 == 0 {
							mu.Lock()
							printer.File(result)
							mu.Unlock()
							continue
						}
						if writeErr := os.WriteFile(entry.Path, result.After, info.Mode()); writeErr != nil {
							atomic.AddInt32(&errors, 1)
							mu.Lock()
							printer.Error(entry.Path, writeErr)
							mu.Unlock()
							continue
						}
					}
					atomic.AddInt32(&changed, 1)
				}

				mu.Lock()
				printer.File(result)
				mu.Unlock()
			}
		}()
	}

	for _, e := range entries {
		work <- e
	}
	close(work)
	wg.Wait()

	printer.Summary(int(changed), int(skipped), int(errors), int(total))
	return nil
}

func loadConfig(root string) (config.Config, error) {
	if flagNoConfig {
		return config.Config{}, nil
	}
	path := flagConfigPath
	if path == "" {
		path = config.Discover(root)
	}
	if path == "" {
		return config.Config{}, nil
	}
	return config.Load(path)
}

func parseLangs(flagValue string, cfgLangs []string) []string {
	if flagValue != "" {
		return strings.Split(flagValue, ",")
	}
	return cfgLangs
}

func resolveMaxFileSize(cmd *cobra.Command, cfgValue int64) int64 {
	if cmd.Flags().Changed("max-file-size") {
		return flagMaxFileSize
	}
	if cfgValue > 0 {
		return cfgValue
	}
	return flagMaxFileSize
}

func mergeStrings(cfgVals, flagVals []string) []string {
	if len(cfgVals) == 0 {
		return flagVals
	}
	if len(flagVals) == 0 {
		return cfgVals
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(cfgVals)+len(flagVals))
	for _, v := range append(cfgVals, flagVals...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
