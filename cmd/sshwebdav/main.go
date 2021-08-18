package main

import (
	"fmt"
	"net/url"
	"os"

	"github.com/lima-vm/sshwebdav/pkg/sshwebdav"
	"github.com/lima-vm/sshwebdav/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	if err := newApp().Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

const usageText = "sshwebdav ssh://foo@example.com:22/home/foo http://127.0.0.1:8080/"

func newApp() *cli.App {
	app := cli.NewApp()
	app.Name = "sshwebdav"
	app.Version = version.Version
	app.Usage = "WebDAV server for SSH"
	app.UsageText = usageText
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "debug mode",
		},
		&cli.StringFlag{
			Name:    "ssh-config",
			Aliases: []string{"F"},
			Usage:   "ssh config file",
		},
		&cli.StringFlag{
			Name:    "ssh-identity",
			Aliases: []string{"i"},
			Usage:   "ssh identify file (private key)",
		},
		&cli.StringSliceFlag{
			Name:    "ssh-option",
			Aliases: []string{"o"},
			Usage:   "ssh option (KEY=VAL)",
		},
	}
	app.HideHelpCommand = true
	app.Before = func(clicontext *cli.Context) error {
		if clicontext.Bool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.Action = action
	return app
}

func action(clicontext *cli.Context) error {
	if clicontext.NArg() < 2 {
		return fmt.Errorf("missing args (usage: %s)", usageText)
	}
	if clicontext.NArg() > 2 {
		return fmt.Errorf("too many args (usage: %s)", usageText)
	}
	sshURLStr := clicontext.Args().Get(0)
	sshURL, err := url.Parse(sshURLStr)
	if err != nil {
		return fmt.Errorf("failed to parse SSH URL %q: %w", sshURLStr, err)
	}
	if sshURL.Scheme != "ssh" {
		return fmt.Errorf("the first argument should be like \"ssh://foo@example.com:22/home/foo\", got %q", sshURLStr)
	}

	webdavURLStr := clicontext.Args().Get(1)
	webdavURL, err := url.Parse(webdavURLStr)
	if err != nil {
		return fmt.Errorf("failed to parse WebDAV URL %q: %w", webdavURLStr, err)
	}
	if webdavURL.Scheme != "http" {
		// TODO: https
		return fmt.Errorf("the second argument should be like \"http://127.0.0.1:8080\", got %q", webdavURLStr)
	}

	x, err := sshwebdav.New(sshURL, webdavURL,
		sshwebdav.WithSSHConfig(clicontext.String("ssh-config")),
		sshwebdav.WithSSHIdentity(clicontext.String("ssh-identity")),
		sshwebdav.WithSSHOptions(clicontext.StringSlice("ssh-options")))
	if err != nil {
		return err
	}

	logrus.Info("Hint: open Finder => `Go` => `Connect to Server` to connect.")
	return x.Serve()
}
