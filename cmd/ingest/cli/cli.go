/*
Copyright 2022 The nagare media authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mattn/go-isatty"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/nagare-media/ingest/pkg/config"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/controllers"
	"github.com/nagare-media/ingest/pkg/version"
)

type cli struct {
	configured bool
	configErr  error

	flagSet          *pflag.FlagSet
	printUsageFlag   bool
	printVersionFlag bool
	devModeFlag      bool
	logLevelFlag     string
	configFlag       string

	logger *zap.SugaredLogger
	viper  *viper.Viper
}

// New creates a new command instance
func New() *cli {
	c := &cli{
		configured: false,
		flagSet:    pflag.NewFlagSet("ingest", pflag.ContinueOnError),
		viper:      viper.New(),
	}

	c.flagSet.BoolVarP(&c.printUsageFlag, "help", "h", false, "Print this help message and exit")
	c.flagSet.BoolVarP(&c.printVersionFlag, "version", "V", false, "Print the version number and exit")
	c.flagSet.BoolVar(&c.devModeFlag, "dev", false, "Run in developer mode")
	c.flagSet.StringVarP(&c.logLevelFlag, "log-level", "l", "", "Log level (\"debug\", \"info\", \"warn\", \"error\", \"panic\", \"fatal\")")
	c.flagSet.StringVarP(&c.configFlag, "config", "c", "", "Path to the config file")

	c.viper.SetConfigName("config")
	c.viper.SetConfigType("yaml")
	c.viper.AddConfigPath(".")
	c.viper.AddConfigPath("/etc/nagare-media/ingest/")

	return c
}

// ParseArgs given to this command
func (c *cli) ParseArgs(args []string) {
	c.configured = true

	c.configErr = c.flagSet.Parse(args)
	if c.configErr != nil {
		return
	}

	ll := zap.NewAtomicLevel()
	if c.logLevelFlag == "" {
		if c.devModeFlag {
			c.logLevelFlag = "debug"
		} else {
			c.logLevelFlag = "info"
		}
	}
	c.configErr = ll.UnmarshalText([]byte(c.logLevelFlag))
	if c.configErr != nil {
		return
	}

	var l *zap.Logger
	l, c.configErr = newLoggerConfig(ll, c.devModeFlag).Build()
	if c.configErr != nil {
		return
	}
	c.logger = l.Sugar()
}

// Execute this command
func (c *cli) Execute(ctx context.Context) error {
	if !c.configured {
		c.configErr = errors.New("ingest command not configured")
	}

	if c.configErr != nil {
		fmt.Println(c.configErr)
		fmt.Println()
		c.PrintUsage()
		return c.configErr
	}

	if c.printUsageFlag {
		c.PrintUsage()
		return nil
	}

	if c.printVersionFlag {
		_ = version.Ingest.Write(os.Stdout)
		return nil
	}

	log := c.logger
	defer func() {
		_ = c.logger.Sync()
	}()

	log.Infow("start nagare media ingest", "version", version.Ingest.String())
	var err error

	if c.configFlag == "" {
		log.Debug("search for nagare media ingest server config")
		err = c.viper.ReadInConfig()
	} else {
		log.Debugw("open config file", "config", c.configFlag)

		var f *os.File
		f, err = os.Open(c.configFlag)
		if err != nil {
			log.Errorw("opening config file failed", "error", err)
			return err
		}

		err = c.viper.ReadConfig(f)
	}
	if err != nil {
		log.Errorw("reading config failed", "error", err)
		return err
	}

	cfg := &v1alpha1.Config{}
	err = config.UnmarshalExact(c.viper, cfg)
	if err != nil {
		log.Errorw("unmarshaling config failed", "error", err)
		return err
	}

	// TODO: log configuration

	// handle termination
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()

	// create ingest controller
	ctrl, err := controllers.NewIngestController(*cfg)
	if err != nil {
		log.Errorw("initializing ingest failed", "error", err)
		return err
	}

	// start execution
	execCtx := (&controllers.ExecCtx{}).WithLogger(log)
	return ctrl.Exec(ctx, execCtx)
}

// PrintUsage of this command
func (c *cli) PrintUsage() {
	fmt.Println("Usage: ingest [options]")
	c.flagSet.PrintDefaults()
}

func newLoggerConfig(level zap.AtomicLevel, development bool) zap.Config {
	var levelEncoder zapcore.LevelEncoder
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		levelEncoder = zapcore.LowercaseColorLevelEncoder
	} else {
		levelEncoder = zapcore.LowercaseLevelEncoder
	}

	return zap.Config{
		Level:             level,
		Development:       development,
		DisableStacktrace: true,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding: "console",
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:    "M",
			LevelKey:      "L",
			TimeKey:       "T",
			NameKey:       "N",
			CallerKey:     "C",
			FunctionKey:   zapcore.OmitKey,
			StacktraceKey: "S",

			EncodeLevel:    levelEncoder,
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
}
