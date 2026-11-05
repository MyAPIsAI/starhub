package staffcli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"

	"starhub/config"
	"starhub/model"
)

// Run keeps bootstrap and sole-Root recovery outside the public registration API.
func Run(args []string, getenv func(string) string, out io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != "staff" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: staff init|reset-2fa|install-token [--username NAME] [--config PATH]")
	}
	flags := flag.NewFlagSet("staff", flag.ContinueOnError)
	flags.SetOutput(out)
	username := flags.String("username", "root", "staff account name")
	file := flags.String("config", "", "startup configuration")
	if err := flags.Parse(args[2:]); err != nil {
		return true, err
	}
	if flags.NArg() != 0 || !regexp.MustCompile(`^[A-Za-z0-9_.-]{3,64}$`).MatchString(*username) {
		return true, errors.New("invalid staff command arguments")
	}
	startupArgs := []string{}
	if *file != "" {
		startupArgs = []string{"--config", *file}
	}
	startup, err := config.LoadStartup(startupArgs, getenv)
	if err != nil {
		return true, err
	}
	if err := config.ApplyStartup(startup); err != nil {
		return true, err
	}
	if err := model.InitDB(startup.Bootstrap.DBPath); err != nil {
		return true, err
	}
	defer model.CloseDB()
	switch args[1] {
	case "init":
		err = model.InitializeStaffRoot(*username, getenv("STARHUB_STAFF_PASSWORD"), "", false)
	case "reset-2fa":
		err = model.ResetSoleRootOTP(*username)
	case "install-token":
		if !config.StaffOpenSourceSetup() {
			return true, errors.New("installation tokens require open-source setup mode")
		}
		var token string
		token, err = model.NewStaffInstallationToken()
		if err == nil {
			fmt.Fprintln(out, "One-time installation token:", token)
			return true, nil
		}
	default:
		return true, errors.New("unknown staff command")
	}
	if err == nil {
		fmt.Fprintln(out, "Staff operation completed; account and Key credentials were not printed.")
	}
	return true, err
}
