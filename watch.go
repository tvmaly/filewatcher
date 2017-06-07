package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/dnephin/filewatcher/files"
	"github.com/dnephin/filewatcher/runner"
	"github.com/dnephin/filewatcher/ui"
	flag "github.com/spf13/pflag"
	fsnotify "gopkg.in/fsnotify.v1"
)

type options struct {
	verbose bool
	quiet   bool
	exclude []string
	dirs    []string
	depth   int
	command []string
}

func watch(watcher *fsnotify.Watcher, runner *runner.Runner, ui *ui.UI) error {
	for {
		select {
		case event := <-watcher.Events:
			log.Debugf("Event: %s", event)

			if isNewDir(event, runner.Excludes()) {
				log.Debugf("Watching new directory: %s", event.Name)
				watcher.Add(event.Name)
				continue
			}

			start := time.Now()
			handled, err := runner.HandleEvent(event)
			if !handled {
				continue
			}
			// TODO: handle formatting somewhere else, and add color
			elapsed := time.Since(start)
			msg := "OK"
			if err != nil {
				msg = err.Error()
			}
			ui.Footer(fmt.Sprintf("%s │ %s", msg, elapsed))
		case err := <-watcher.Errors:
			return err
		}
	}
}

func isNewDir(event fsnotify.Event, exclude *files.ExcludeList) bool {
	if event.Op&fsnotify.Create != fsnotify.Create {
		return false
	}

	fileInfo, err := os.Stat(event.Name)
	if err != nil {
		log.Warnf("Failed to stat %s: %s", event.Name, err)
		return false
	}

	return fileInfo.IsDir() && !exclude.IsMatch(event.Name)
}

func buildWatcher(dirs []string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	log.Infof("Watching directories: %s", strings.Join(dirs, ", "))
	for _, dir := range dirs {
		log.Debugf("Adding new watch: %s", dir)
		if err = watcher.Add(dir); err != nil {
			return nil, err
		}
	}
	return watcher, nil
}

func setupFlags() options {
	opts := options{}
	flag.BoolVarP(&opts.verbose, "verbose", "v", false, "Verbose")
	flag.BoolVarP(&opts.quiet, "quiet", "q", false, "Quiet")
	flag.StringSliceVarP(&opts.exclude, "exclude", "x", nil, "Exclude file patterns")
	flag.StringSliceVarP(&opts.dirs, "directory", "d", []string{"."}, "Directories to watch")
	flag.IntVarP(&opts.depth, "depth", "L", 5, "Descend only level directories deep")
	return opts
}

func main() {
	opts := setupFlags()
	cmd := flag.CommandLine
	cmd.Init(os.Args[0], flag.ExitOnError)
	cmd.SetInterspersed(false)
	flag.Usage = func() {
		out := os.Stderr
		fmt.Fprintf(out, "Usage:\n  %s [OPTIONS] COMMAND ARGS... \n\n", os.Args[0])
		fmt.Fprint(out, "Options:\n")
		cmd.PrintDefaults()
	}
	flag.Parse()
	setupLogging(opts)

	opts.command = flag.Args()
	if len(opts.command) == 0 {
		log.Fatalf("A command argument is required.")
	}
	run(opts)
}

func run(opts options) {
	// TODO: flag to disable UI
	ui, err := ui.SetupUI()
	if err != nil {
		log.Fatalf("Error seting up UI: %s", err)
	}
	defer ui.Reset()
	log.SetOutput(ui.Output())
	log.SetFormatter(&log.TextFormatter{ForceColors: true})
	ui.Header(opts.command)

	excludeList, err := files.NewExcludeList(opts.exclude)
	if err != nil {
		log.Fatalf("Error creating exclude list: %s", err)
	}

	watcher, err := buildWatcher(files.WalkDirectories(opts.dirs, opts.depth, excludeList))
	if err != nil {
		log.Fatalf("Error setting up watcher: %s", err)
	}
	defer watcher.Close()

	streams := runner.Streams{Out: ui.Output(), Err: ui.Output()}
	runner := runner.NewRunner(excludeList, opts.command, streams)
	if err = watch(watcher, runner, ui); err != nil {
		log.Fatalf("Error during watch: %s", err)
	}
}

func setupLogging(opts options) {
	if opts.verbose {
		log.SetLevel(log.DebugLevel)
	}
	if opts.quiet {
		log.SetLevel(log.WarnLevel)
	}
}
