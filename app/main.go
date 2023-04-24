package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatih/color"
	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/simplotask/app/config"
	"github.com/umputun/simplotask/app/remote"
	"github.com/umputun/simplotask/app/runner"
)

type options struct {
	TaskFile   string `short:"f" long:"file" description:"task file" default:"spt.yml"`
	TaskName   string `short:"n" long:"name" description:"task name" default:"default"`
	TargetName string `short:"t" long:"target" description:"target name" default:"default"`
	Concurrent int    `short:"c" long:"concurrent" description:"concurrent tasks" default:"1"`

	// target overrides
	TargetHosts   []string `short:"h" long:"host" description:"destination host"`
	InventoryFile string   `short:"i" long:"inventory" description:"inventory file"`
	InventoryHTTP string   `short:"H" long:"inventory-http" description:"inventory http url"`

	// connection overrides
	SSHUser string `short:"u" long:"user" description:"ssh user"`
	SSHKey  string `short:"k" long:"key" description:"ssh key" default:"~/.ssh/id_rsa"`

	Skip []string `short:"s" long:"skip" description:"skip commands"`
	Only []string `short:"o" long:"only" description:"run only commands"`

	Dbg bool `long:"dbg" description:"debug mode"`
	Dev bool `long:"dev" description:"development mode"`
}

var revision = "latest"

func main() {
	fmt.Printf("simplotask %s\n", revision)

	var opts options
	p := flags.NewParser(&opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	if _, err := p.Parse(); err != nil {
		if err.(*flags.Error).Type != flags.ErrHelp {
			fmt.Printf("%v", err)
		}
		os.Exit(1)
	}
	setupLog(opts.Dbg, opts.Dev)

	if err := run(opts); err != nil {
		log.Panicf("[ERROR] %v", err)
	}
}

func run(opts options) error {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) // cancel on SIGINT or SIGTERM
	go func() {
		sig := <-sigs
		log.Printf("received signal: %v", sig)
		cancel()
	}()

	conf, err := config.New(opts.TaskFile,
		&config.Overrides{TargetHosts: opts.TargetHosts, InventoryFile: opts.InventoryFile, InventoryHTTP: opts.InventoryHTTP})
	if err != nil {
		return fmt.Errorf("can't read config: %w", err)
	}

	connector, err := remote.NewConnector(sshUserAndKey(opts, conf))
	if err != nil {
		return fmt.Errorf("can't create connector: %w", err)
	}
	r := runner.Process{
		Concurrency: opts.Concurrent,
		Connector:   connector,
		Config:      conf,
		Only:        opts.Only,
		Skip:        opts.Skip,
	}
	return r.Run(ctx, opts.TaskName, opts.TargetName)
}

func sshUserAndKey(opts options, conf *config.PlayBook) (user, key string) {
	sshUser := conf.User // default to global config user
	if tsk, ok := conf.Tasks[opts.TaskName]; ok && tsk.User != "" {
		sshUser = tsk.User // override with task config
	}
	if opts.SSHUser != "" { // override with command line
		sshUser = opts.SSHUser
	}

	sshKey := conf.SSHKey  // default to global config key
	if opts.SSHKey != "" { // override with command line
		sshKey = opts.SSHKey
	}
	return sshUser, sshKey
}

func setupLog(dbg, dev bool) {
	logOpts := []lgr.Option{lgr.Out(io.Discard), lgr.Err(io.Discard)} // default to discard
	if dbg {
		// debug mode shows all messages but no caller/stack trace
		logOpts = []lgr.Option{lgr.Debug, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}
	if dev {
		// dev mode shows all messages with caller/stack trace
		logOpts = []lgr.Option{lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}

	colorizer := lgr.Mapper{
		ErrorFunc:  func(s string) string { return color.New(color.FgHiRed).Sprint(s) },
		WarnFunc:   func(s string) string { return color.New(color.FgRed).Sprint(s) },
		InfoFunc:   func(s string) string { return color.New(color.FgYellow).Sprint(s) },
		DebugFunc:  func(s string) string { return color.New(color.FgWhite).Sprint(s) },
		CallerFunc: func(s string) string { return color.New(color.FgBlue).Sprint(s) },
		TimeFunc:   func(s string) string { return color.New(color.FgCyan).Sprint(s) },
	}
	logOpts = append(logOpts, lgr.Map(colorizer))

	lgr.SetupStdLogger(logOpts...)
	lgr.Setup(logOpts...)
}