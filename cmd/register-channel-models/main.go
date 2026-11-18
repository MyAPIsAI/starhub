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
	database := flag.String("db", "", "Existing database path; otherwise use server configuration")
	flag.Parse()
	if err := operator.OpenDatabase(*database); err != nil {
		fmt.Fprintln(os.Stderr, "Database open failed:", err)
		os.Exit(1)
	}
	defer model.CloseDB()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	inserted, err := model.BackfillChannelModelMetadata(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Model registration failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Registered %d missing channel models; existing metadata and prices were retained.\n", inserted)
}
