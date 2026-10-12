package cli

import (
	"flag"
	"fmt"
	"io"
)

// Standard process exit codes.
const (
	ExitSuccess    = 0
	ExitGeneralErr = 1
	ExitUsageError = 2
)

const appDescription = "Local-first personal computer memory system for macOS"

// Run executes the root CLI application with the given arguments and I/O streams.
// It returns an integer process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "version":
			fmt.Fprintln(stdout, VersionString())
			return ExitSuccess
		case "help":
			printUsage(stdout)
			return ExitSuccess
		case "init":
			return runInit(stdout, stderr)
		case "status":
			return runStatus(stdout, stderr)
		case "doctor":
			return runDoctor(stdout, stderr)
		case "watch":
			return runWatch(args[1:], stdout, stderr)
		case "events":
			return runEvents(args[1:], stdout, stderr)
		case "git":
			return runGit(args[1:], stdout, stderr)
		case "repos":
			return runRepos(stdout, stderr)
		case "terminal":
			return runTerminal(args[1:], stdout, stderr)
		case "search":
			return runSearch(args[1:], stdout, stderr)
		case "timeline":
			return runTimeline(args[1:], stdout, stderr)
		case "resume":
			return runResume(args[1:], stdout, stderr)
		case "package":
			return runPackage(args[1:], stdout, stderr)
		case "projects":
			return runProjects(args[1:], stdout, stderr)
		case "export":
			return runExport(args[1:], stdout, stderr)
		case "checkpoint":
			return runCheckpoint(args[1:], stdout, stderr)
		case "daemon":
			return runDaemon(args[1:], stdout, stderr)
		}
	}

	fs := flag.NewFlagSet(AppName, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "display application name and version")
	fs.BoolVar(&showVersion, "v", false, "display application name and version (shorthand)")

	fs.Usage = func() {
		printUsage(stdout)
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return ExitSuccess
		}
		fmt.Fprintf(stderr, "\nRun '%s --help' for usage details.\n", AppName)
		return ExitUsageError
	}

	if showVersion {
		fmt.Fprintln(stdout, VersionString())
		return ExitSuccess
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unknown command: %s\n\nRun '%s --help' for usage details.\n", fs.Arg(0), AppName)
		return ExitUsageError
	}

	fmt.Fprintf(stdout, "%s\n\n%s\n\nRun '%s --help' for available commands and options.\n",
		VersionString(), appDescription, AppName)
	return ExitSuccess
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, "%s\n\n", VersionString())
	fmt.Fprintf(w, "%s\n\n", appDescription)
	fmt.Fprintf(w, "Usage:\n  %s [flags]\n  %s [command]\n\n", AppName, AppName)
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  projects   Overview of known projects, recorded activity, and health\n")
	fmt.Fprintf(w, "  resume     Synthesize project-resumption context and activity sessions\n")
	fmt.Fprintf(w, "  package    Package project resumption context into portable bundle\n")
	fmt.Fprintf(w, "  checkpoint Save, list, view, and delete project resumption checkpoints\n")
	fmt.Fprintf(w, "  export     Export project-resumption context to Markdown or JSON\n")
	fmt.Fprintf(w, "  search     Unified cross-domain full-text search across all memory\n")
	fmt.Fprintf(w, "  timeline   Chronological timeline of all observed computer activity\n")
	fmt.Fprintf(w, "  git        Observe, scan, and inspect Git repositories and history\n")
	fmt.Fprintf(w, "  repos      List all registered Git repositories\n")
	fmt.Fprintf(w, "  terminal   Record, query, and manage opt-in terminal execution history\n")
	fmt.Fprintf(w, "  watch      Monitor directory for filesystem events\n")
	fmt.Fprintf(w, "  events     Query and inspect recorded filesystem events\n")
	fmt.Fprintf(w, "  daemon     Run supervised local background daemon with secure IPC\n")
	fmt.Fprintf(w, "  init       Initialize local storage directories and database schema\n")
	fmt.Fprintf(w, "  status     Display application and storage status\n")
	fmt.Fprintf(w, "  doctor     Run non-destructive diagnostic checks\n")
	fmt.Fprintf(w, "  help       Show command-line usage\n")
	fmt.Fprintf(w, "  version    Show application version\n\n")
	fmt.Fprintf(w, "Workflow:\n")
	fmt.Fprintf(w, "  %s projects                         # 1. Identify active or dormant projects\n", AppName)
	fmt.Fprintf(w, "  %s resume <path>                    # 2. Review synthesized resumption report\n", AppName)
	fmt.Fprintf(w, "  %s package <path> -o pkg.md         # 3. Export portable context package\n", AppName)
	fmt.Fprintf(w, "  %s checkpoint create <path> -label  # 4. Save immutable historical snapshot\n\n", AppName)
	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -h, --help       Show command-line usage\n")
	fmt.Fprintf(w, "  -v, --version    Show application name and version\n")
}
