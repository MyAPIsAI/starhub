package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"starhub/internal/operator"
	"starhub/model"
)

func main() {
	database := flag.String("db", "", "Existing SQLite database path; otherwise use server configuration")
	directory := flag.String("directory", "", "Existing snapshot directory")
	source := flag.String("snapshot", "", "Standalone immutable SQLite snapshot to verify or restore")
	restore := flag.String("restore-to", "", "New database path; existing files are never overwritten")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var err error
	if *source != "" {
		if *restore != "" {
			err = model.RestoreSQLiteSnapshot(ctx, *source, *restore)
		} else {
			err = model.VerifySQLiteSnapshot(ctx, *source)
		}
	} else if *restore != "" || *directory == "" {
		err = fmt.Errorf("directory or snapshot required")
	} else {
		err = operator.OpenDatabase(*database)
		if err == nil {
			defer model.CloseDB()
			var path string
			path, err = model.CreateSQLiteBackup(ctx, *directory)
			if err == nil {
				fmt.Println(path)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "backup operation failed:", err)
		os.Exit(1)
	}
	fmt.Println("Snapshot operation completed. Stop the service before switching database files.")
}
