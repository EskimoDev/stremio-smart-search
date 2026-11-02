// Command sss-bench measures Stremio Smart Search and compares two builds.
//
//	go run ./tools/sss-bench run --profile quick
//	go run ./tools/sss-bench ab --baseline ../sss-base --candidate . --profile standard
//	go run ./tools/sss-bench compare --baseline before.json --candidate after.json
//
// It runs the real application and two small probes in fresh child processes,
// each in a private temporary folder on a private port, against a generated
// fixture or an explicitly supplied snapshot. It never downloads anything and
// never touches an installed helper, its settings or its catalogue. See
// docs/BENCHMARKING.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Exit codes. Agents and CI should act on these, not on terminal text.
const (
	exitOK            = 0 // finished; for compare, no regression
	exitFailure       = 1 // the harness could not produce a result
	exitInput         = 2 // bad flags or unusable input; nothing was measured
	exitIncomplete    = 3 // finished, but part of the measurement failed
	exitRegression    = 4 // compare found a regression
	exitNotComparable = 5 // compare refused: the results measure different things
)

const usage = `sss-bench measures Stremio Smart Search and compares two builds.

Commands:
  run       Measure one build                    sss-bench run --profile quick
  ab        Measure two builds, interleaved      sss-bench ab --baseline ../sss-base --candidate .
  compare   Compare two saved results            sss-bench compare --baseline a.json --candidate b.json
  report    Print a saved result as Markdown     sss-bench report --input a.json
  metrics   List profiles and metrics            sss-bench metrics --json
  version   Print the harness version

Profiles: quick (default, about 30 s), standard (about 4 min), full (about 16 min).
Run "sss-bench <command> --help" for flags. Exit codes: 0 ok, 1 harness failure,
2 bad input, 3 incomplete, 4 regression, 5 not comparable.
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitInput
	}
	command, rest := args[0], args[1:]
	switch command {
	case "run":
		return commandRun(rest, stdout, stderr)
	case "ab":
		return commandAB(rest, stdout, stderr)
	case "compare":
		return commandCompare(rest, stdout, stderr)
	case "report":
		return commandReport(rest, stdout, stderr)
	case "metrics":
		return commandMetrics(rest, stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "sss-bench %s (result schema %d, measurement %d, workload %d)\n", harnessVersion, schemaVersion, measurementVersion, workloadVersion)
		return exitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	fmt.Fprintf(stderr, "sss-bench: unknown command %q\n\n%s", command, usage)
	return exitInput
}

func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("sss-bench "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

// parse reports a usage error's exit code, and rejects stray arguments, which
// usually mean a mistyped flag that would otherwise be silently ignored.
func parse(flags *flag.FlagSet, args []string, stderr io.Writer) (int, bool) {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK, false
		}
		return exitInput, false
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "sss-bench: unexpected argument %q\n", flags.Arg(0))
		return exitInput, false
	}
	return 0, true
}

// tuning holds the overrides shared by run and ab. Negative means keep the
// profile's value.
type tuning struct {
	profile                     string
	reps, passes, titles        int
	settle, throughput, sustain time.Duration
	catalog, imdbDir, repo      string
	label, workDir              string
	keepWork, force, json       bool
	quiet                       bool
}

func addTuning(flags *flag.FlagSet, t *tuning) {
	flags.StringVar(&t.profile, "profile", "quick", "quick, standard or full")
	flags.IntVar(&t.reps, "reps", -1, "fresh-process repetitions per target, 1 to 30")
	flags.IntVar(&t.passes, "passes", -1, "measured passes over the queries in each repetition")
	flags.IntVar(&t.titles, "titles", -1, "size of the generated fixture")
	flags.DurationVar(&t.settle, "settle", -1, "wait before reading idle memory")
	flags.DurationVar(&t.throughput, "throughput", -1, "duration of each throughput level")
	flags.DurationVar(&t.sustain, "sustain", -1, "duration of sustained searching for memory growth")
	flags.StringVar(&t.catalog, "catalog", "", "existing catalogue snapshot to measure instead of the fixture; it is copied, never changed")
	flags.StringVar(&t.imdbDir, "imdb-dir", "", "folder of local title.ratings and title.basics .tsv.gz files for full's catalogue operations; nothing is downloaded")
	flags.StringVar(&t.repo, "repo", "", "repository holding the quality cases (default: found from the current folder)")
	flags.StringVar(&t.label, "label", "", "free text recorded with the result")
	flags.StringVar(&t.workDir, "work-dir", "", "parent folder for temporary files (default: the system's)")
	flags.BoolVar(&t.keepWork, "keep-work", false, "keep the temporary folder for inspection")
	flags.BoolVar(&t.force, "force", false, "allow overwriting existing output files")
	flags.BoolVar(&t.json, "json", false, "print the result JSON on stdout")
	flags.BoolVar(&t.quiet, "quiet", false, "no progress on stderr")
}

// config validates the shared flags. Every check here runs before any work.
func (t tuning) config() (Config, error) {
	p, ok := profileByName(t.profile)
	if !ok {
		return Config{}, fmt.Errorf("unknown profile %q: choose quick, standard or full", t.profile)
	}
	if t.reps != -1 {
		if t.reps < 1 || t.reps > 30 {
			return Config{}, errors.New("--reps must be between 1 and 30")
		}
		p.Repetitions = t.reps
	}
	if t.passes != -1 {
		if t.passes < 1 {
			return Config{}, errors.New("--passes must be positive")
		}
		p.Passes = t.passes
	}
	if t.titles != -1 {
		if t.catalog != "" {
			return Config{}, errors.New("--titles sizes the generated fixture, so it cannot be used with --catalog")
		}
		if t.titles < 1000 {
			return Config{}, errors.New("--titles must be at least 1000")
		}
		p.Titles = t.titles
	}
	for _, d := range []struct {
		name  string
		value time.Duration
		into  *time.Duration
	}{{"--settle", t.settle, &p.Settle}, {"--throughput", t.throughput, &p.Throughput}, {"--sustain", t.sustain, &p.Sustain}} {
		if d.value == -1 {
			continue
		}
		if d.value < 0 {
			return Config{}, fmt.Errorf("%s must not be negative", d.name)
		}
		*d.into = d.value
	}
	if len(p.Concurrency) > 0 && p.Throughput <= 0 {
		return Config{}, errors.New("--throughput must be positive for this profile")
	}
	if p.Sustain > 0 && p.Sustain < 3*time.Second {
		return Config{}, errors.New("--sustain needs at least 3s to see any trend; use 0 to skip it")
	}
	repo := t.repo
	var err error
	if repo == "" {
		repo = "."
	}
	if repo, err = findRepoRoot(repo); err != nil {
		return Config{}, err
	}
	cfg := Config{Profile: p, Catalog: t.catalog, IMDbDir: t.imdbDir, Repo: repo, Label: t.label, WorkDir: t.workDir, KeepWork: t.keepWork}
	if t.catalog != "" {
		if info, err := os.Stat(t.catalog); err != nil || info.IsDir() {
			return Config{}, fmt.Errorf("--catalog %s is not a readable file", t.catalog)
		}
	}
	if t.imdbDir != "" {
		if info, err := os.Stat(t.imdbDir); err != nil || !info.IsDir() {
			return Config{}, fmt.Errorf("--imdb-dir %s is not a folder", t.imdbDir)
		}
		if p.tier < tierFull {
			return Config{}, errors.New("--imdb-dir is only used by the full profile")
		}
	}
	return cfg, nil
}

func resolveTarget(name, source, binary, defaultSource string) (targetSpec, error) {
	spec := targetSpec{name: name}
	if source != "" && binary != "" {
		return spec, errors.New("choose --source or --binary, not both")
	}
	if binary != "" {
		info, err := os.Stat(binary)
		if err != nil || info.IsDir() {
			return spec, fmt.Errorf("--binary %s is not a file", binary)
		}
		spec.binary = binary
		return spec, nil
	}
	if source == "" {
		source = defaultSource
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return spec, err
	}
	if _, err := os.Stat(filepath.Join(absolute, "cmd", "smart-search", "main.go")); err != nil {
		return spec, fmt.Errorf("%s is not a Smart Search source tree (no cmd/smart-search)", source)
	}
	spec.source = absolute
	return spec, nil
}

// outputs is where one result's files go.
type outputs struct{ json, markdown, bench string }

func outputsFor(base string) outputs {
	base = strings.TrimSuffix(base, ".json")
	return outputs{base + ".json", base + ".md", base + ".bench"}
}

// defaultResultsDir sits beside the repository: reports may hold title names, and
// the project keeps those out of its tree.
func defaultResultsDir(repo string) string {
	return filepath.Join(filepath.Dir(repo), "sss-bench-results")
}

// claim checks a run's outputs before it starts, so a long run never ends unable
// to save, and never replaces a saved baseline by accident.
func (o outputs) claim(force bool) error {
	if err := os.MkdirAll(filepath.Dir(o.json), 0o755); err != nil {
		return err
	}
	if force {
		return nil
	}
	for _, path := range []string{o.json, o.markdown, o.bench} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; choose another name or pass --force", path)
		}
	}
	return nil
}

// writeAtomic never leaves a half-written report behind.
func writeAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".sss-bench-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

func (o outputs) save(r *Result) error {
	data, err := marshal(r)
	if err != nil {
		return err
	}
	if err := writeAtomic(o.json, data); err != nil {
		return err
	}
	if err := writeAtomic(o.markdown, []byte(renderRun(r))); err != nil {
		return err
	}
	return writeAtomic(o.bench, []byte(renderBench(r)))
}

func exitFor(status string) int {
	switch status {
	case statusComplete:
		return exitOK
	case statusPartial:
		return exitIncomplete
	}
	return exitFailure
}

func interrupted() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func commandRun(args []string, stdout, stderr io.Writer) int {
	flags := newFlags("run", stderr)
	var t tuning
	addTuning(flags, &t)
	source := flags.String("source", "", "source tree to build and measure (default: this repository)")
	binary := flags.String("binary", "", "prebuilt application to measure; engine metrics are then unavailable")
	out := flags.String("out", "", "result file; .md and .bench are written beside it")
	if code, ok := parse(flags, args, stderr); !ok {
		return code
	}
	cfg, err := t.config()
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	spec, err := resolveTarget("target", *source, *binary, cfg.Repo)
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	cfg.Sources = []string{spec.source}
	runID, err := randomID()
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitFailure
	}
	base := *out
	if base == "" {
		base = filepath.Join(defaultResultsDir(cfg.Repo), time.Now().Format("20060102-150405")+"-"+cfg.Profile.Name)
	}
	files := outputsFor(base)
	if err := files.claim(t.force); err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	ctx, stop := interrupted()
	defer stop()
	log := &logger{out: stderr, quiet: t.quiet}
	s, err := newSession(ctx, cfg, log)
	if err != nil {
		return reportSetup(err, stderr)
	}
	defer s.close()
	result := s.run(ctx, spec, runID)
	if err := files.save(result); err != nil {
		fmt.Fprintln(stderr, "sss-bench: could not save the result:", err)
		return exitFailure
	}
	printRun(result, files, t.json, stdout, stderr)
	return exitFor(result.Status)
}

func reportSetup(err error, stderr io.Writer) int {
	fmt.Fprintln(stderr, "sss-bench:", err)
	var input inputError
	if errors.As(err, &input) {
		return exitInput
	}
	return exitFailure
}

func printRun(r *Result, files outputs, asJSON bool, stdout, stderr io.Writer) {
	if asJSON {
		data, _ := marshal(r)
		stdout.Write(data)
	} else {
		fmt.Fprintf(stdout, "%s: %s\n%s\n%s\n%s\n", r.Workload.Profile, r.Status, files.json, files.markdown, files.bench)
	}
	for _, p := range r.Errors {
		fmt.Fprintf(stderr, "sss-bench: error in %s: %s\n", p.Stage, p.Message)
	}
}

func commandAB(args []string, stdout, stderr io.Writer) int {
	flags := newFlags("ab", stderr)
	var t tuning
	addTuning(flags, &t)
	baseline := flags.String("baseline", "", "baseline: a source tree or a prebuilt binary")
	candidate := flags.String("candidate", ".", "candidate: a source tree or a prebuilt binary")
	outDir := flags.String("out-dir", "", "folder for the results (default: a new folder beside the repository)")
	alpha := flags.Float64("alpha", defaultAlpha, "significance level")
	if code, ok := parse(flags, args, stderr); !ok {
		return code
	}
	if *baseline == "" {
		fmt.Fprintln(stderr, "sss-bench: ab needs --baseline; see docs/BENCHMARKING.md for making a worktree of main")
		return exitInput
	}
	if *alpha <= 0 || *alpha >= 1 {
		fmt.Fprintln(stderr, "sss-bench: --alpha must be between 0 and 1")
		return exitInput
	}
	cfg, err := t.config()
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	cfg.Interleaved = true
	spec := func(name, path string) (targetSpec, error) {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return resolveTarget(name, "", path, "")
		}
		return resolveTarget(name, path, "", "")
	}
	first, err := spec("baseline", *baseline)
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	second, err := spec("candidate", *candidate)
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	if first.source != "" && first.source == second.source {
		fmt.Fprintln(stderr, "sss-bench: baseline and candidate are the same folder. For an A/A noise check, use a second worktree")
		return exitInput
	}
	cfg.Sources = []string{first.source, second.source}
	directory := *outDir
	if directory == "" {
		directory = filepath.Join(defaultResultsDir(cfg.Repo), time.Now().Format("20060102-150405")+"-ab-"+cfg.Profile.Name)
	}
	before, after := outputsFor(filepath.Join(directory, "baseline")), outputsFor(filepath.Join(directory, "candidate"))
	comparisonJSON, comparisonMD := filepath.Join(directory, "comparison.json"), filepath.Join(directory, "comparison.md")
	for _, claim := range []error{before.claim(t.force), after.claim(t.force)} {
		if claim != nil {
			fmt.Fprintln(stderr, "sss-bench:", claim)
			return exitInput
		}
	}
	runID, err := randomID()
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitFailure
	}
	ctx, stop := interrupted()
	defer stop()
	s, err := newSession(ctx, cfg, &logger{out: stderr, quiet: t.quiet})
	if err != nil {
		return reportSetup(err, stderr)
	}
	defer s.close()
	baseResult, candidateResult := s.runInterleaved(ctx, first, second, runID)
	if err := before.save(baseResult); err != nil {
		fmt.Fprintln(stderr, "sss-bench: could not save the baseline:", err)
		return exitFailure
	}
	if err := after.save(candidateResult); err != nil {
		fmt.Fprintln(stderr, "sss-bench: could not save the candidate:", err)
		return exitFailure
	}
	comparison := compareResults(baseResult, candidateResult, *alpha)
	return finishComparison(comparison, comparisonJSON, comparisonMD, t.json, stdout, stderr)
}

func finishComparison(c *Comparison, jsonPath, markdownPath string, asJSON bool, stdout, stderr io.Writer) int {
	data, err := marshal(c)
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitFailure
	}
	if jsonPath != "" {
		if err := writeAtomic(jsonPath, data); err != nil {
			fmt.Fprintln(stderr, "sss-bench:", err)
			return exitFailure
		}
	}
	markdown := renderComparison(c)
	if markdownPath != "" {
		if err := writeAtomic(markdownPath, []byte(markdown)); err != nil {
			fmt.Fprintln(stderr, "sss-bench:", err)
			return exitFailure
		}
	}
	if asJSON {
		stdout.Write(data)
	} else {
		fmt.Fprint(stdout, markdown)
	}
	switch {
	case c.Verdict == overallRegressed:
		return exitRegression
	case c.Verdict == overallIncomparable:
		return exitNotComparable
	case c.Incomplete:
		return exitIncomplete
	}
	return exitOK
}

func commandCompare(args []string, stdout, stderr io.Writer) int {
	flags := newFlags("compare", stderr)
	baseline := flags.String("baseline", "", "baseline result JSON")
	candidate := flags.String("candidate", "", "candidate result JSON")
	out := flags.String("out", "", "write the comparison JSON here")
	report := flags.String("report", "", "write the Markdown report here")
	alpha := flags.Float64("alpha", defaultAlpha, "significance level")
	asJSON := flags.Bool("json", false, "print JSON on stdout instead of Markdown")
	if code, ok := parse(flags, args, stderr); !ok {
		return code
	}
	if *baseline == "" || *candidate == "" {
		fmt.Fprintln(stderr, "sss-bench: compare needs --baseline and --candidate")
		return exitInput
	}
	if *alpha <= 0 || *alpha >= 1 {
		fmt.Fprintln(stderr, "sss-bench: --alpha must be between 0 and 1")
		return exitInput
	}
	before, err := readResult(*baseline)
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	after, err := readResult(*candidate)
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	return finishComparison(compareResults(before, after, *alpha), *out, *report, *asJSON, stdout, stderr)
}

func commandReport(args []string, stdout, stderr io.Writer) int {
	flags := newFlags("report", stderr)
	input := flags.String("input", "", "result JSON")
	output := flags.String("output", "", "write Markdown here instead of stdout")
	if code, ok := parse(flags, args, stderr); !ok {
		return code
	}
	if *input == "" {
		fmt.Fprintln(stderr, "sss-bench: report needs --input")
		return exitInput
	}
	result, err := readResult(*input)
	if err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitInput
	}
	text := renderRun(result)
	if *output == "" {
		fmt.Fprint(stdout, text)
		return exitOK
	}
	if err := writeAtomic(*output, []byte(text)); err != nil {
		fmt.Fprintln(stderr, "sss-bench:", err)
		return exitFailure
	}
	return exitOK
}

type metricListing struct {
	ID           string  `json:"id"`
	Group        string  `json:"group"`
	Unit         string  `json:"unit"`
	Better       string  `json:"better"`
	FromProfile  string  `json:"from_profile"`
	Needs        string  `json:"needs,omitempty"`
	Counter      string  `json:"counter,omitempty"`
	Primary      bool    `json:"primary"`
	Available    bool    `json:"available_on_this_platform"`
	MinChangePct float64 `json:"practical_floor_pct,omitempty"`
	MinChangeAbs float64 `json:"practical_floor_abs,omitempty"`
	Definition   string  `json:"definition"`
}

type profileListing struct {
	Name         string   `json:"name"`
	Titles       int      `json:"fixture_titles"`
	Repetitions  int      `json:"repetitions"`
	Passes       int      `json:"passes"`
	Suites       []string `json:"quality_suites"`
	Generated    bool     `json:"generated_quality"`
	CatalogueOps bool     `json:"catalogue_operations"`
	Cost         string   `json:"expected_cost"`
}

func commandMetrics(args []string, stdout, stderr io.Writer) int {
	flags := newFlags("metrics", stderr)
	asJSON := flags.Bool("json", false, "print JSON")
	if code, ok := parse(flags, args, stderr); !ok {
		return code
	}
	tiers := []string{"quick", "standard", "full"}
	listing := struct {
		Profiles []profileListing `json:"profiles"`
		Metrics  []metricListing  `json:"metrics"`
	}{}
	for _, p := range profiles() {
		listing.Profiles = append(listing.Profiles, profileListing{p.Name, p.Titles, p.Repetitions, p.Passes, p.Suites, p.Generated, p.CatalogOps, p.Cost})
	}
	for _, d := range metricDefs() {
		available := !(strings.HasPrefix(d.id, "memory.") && d.counter == "")
		listing.Metrics = append(listing.Metrics, metricListing{d.id, d.group, d.unit, d.better, tiers[d.tier], d.needs, d.counter, d.primary, available, d.minPct, d.minAbs, d.definition})
	}
	if *asJSON {
		data, err := marshal(listing)
		if err != nil {
			fmt.Fprintln(stderr, "sss-bench:", err)
			return exitFailure
		}
		stdout.Write(data)
		return exitOK
	}
	for _, p := range listing.Profiles {
		fmt.Fprintf(stdout, "%-9s %7d titles, %2d repetitions: %s\n", p.Name, p.Titles, p.Repetitions, p.Cost)
	}
	fmt.Fprintln(stdout)
	for _, m := range listing.Metrics {
		note := m.Unit
		if m.Counter != "" {
			note += ", " + m.Counter
		}
		if !m.Available {
			note += ", not available on this platform"
		}
		fmt.Fprintf(stdout, "%-46s %-9s %s\n", m.ID, m.FromProfile, note)
	}
	return exitOK
}
