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
	rotate := flag.Bool("rotate", false, "Rewrap credentials with the configured active key")
	reason := flag.String("reason", "", "Required audit reason when rotating")
	flag.Parse()
	if *database == "" || (*rotate && *reason == "") {
		fmt.Fprintln(os.Stderr, "db and an audit reason for rotation are required")
		os.Exit(2)
	}
	if _, err := os.Stat(*database); err != nil {
		fmt.Fprintln(os.Stderr, "existing database required")
		os.Exit(1)
	}
	identity, err := user.Current()
	if err != nil {
		fmt.Fprintln(os.Stderr, "operator identity unavailable")
		os.Exit(1)
	}
	if err := model.InitDB(*database); err != nil {
		fmt.Fprintln(os.Stderr, "credential storage verification failed")
		os.Exit(1)
	}
	defer model.CloseDB()
	if !*rotate {
		fmt.Println("Credential storage verified; no credential material is displayed.")
		return
	}
	count, err := model.MigrateCredentialStorage(true, identity.Username, *reason)
	if err != nil {
		fmt.Fprintln(os.Stderr, "credential rotation failed and was rolled back")
		os.Exit(1)
	}
	fmt.Printf("Credential rotation committed: %d stored values.\n", count)
}
