package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"

	"starhub/model"
)

func main() {
	database := flag.String("db", "", "Existing SQLite database path")
	action := flag.String("action", "", "initialize or reset-2fa")
	username := flag.String("username", "", "Target staff username")
	reason := flag.String("reason", "", "Required audit reason")
	flag.Parse()
	if *database == "" || *action == "" || *username == "" || *reason == "" {
		fmt.Fprintln(os.Stderr, "db, action, username and reason are required")
		os.Exit(2)
	}
	identity, err := user.Current()
	if err != nil {
		fmt.Fprintln(os.Stderr, "operator identity unavailable")
		os.Exit(1)
	}
	if err := model.InitDB(*database); err != nil {
		fmt.Fprintln(os.Stderr, "database initialization failed")
		os.Exit(1)
	}
	defer model.CloseDB()
	password := os.Getenv("STARHUB_OPERATOR_PASSWORD")
	os.Unsetenv("STARHUB_OPERATOR_PASSWORD")
	if err := model.OperateStaffAccount(*action, *username, password, identity.Username, *reason); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Staff operation committed. Complete security setup and sign in again.")
}
