package main

import (
	config "basaltpass-backend/internal/config"
	"basaltpass-backend/internal/service/basaltimport"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"basaltpass-backend/internal/common"
)

func main() {
	var (
		configPath = flag.String("config", "", "Optional config file path")
		dir        = flag.String("dir", "", "Path to a .basalt directory")
		tenantID   = flag.Uint("tenant-id", 0, "Tenant ID to import into")
		userID     = flag.Uint("user-id", 0, "User ID used as creator/updater")
		dryRun     = flag.Bool("dry-run", false, "Validate and preview without writing to DB")
	)
	flag.Parse()

	if *dir == "" {
		log.Printf("--dir is required")
		return
	}
	if *tenantID == 0 {
		log.Printf("--tenant-id is required")
		return
	}
	if *userID == 0 {
		log.Printf("--user-id is required")
		return
	}

	if _, err := config.Load(*configPath); err != nil {
		log.Printf("load config: %v", err)
		return
	}

	bundle, err := basaltimport.LoadBundleFromDir(*dir)
	if err != nil {
		log.Printf("load bundle: %v", err)
		return
	}

	report, err := basaltimport.ImportBundle(common.DB(), bundle, basaltimport.Options{
		TenantID: uint(*tenantID),
		UserID:   uint(*userID),
		DryRun:   *dryRun,
	})
	if err != nil {
		log.Printf("import bundle: %v", err)
		return
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		log.Printf("encode report: %v", err)
		return
	}

	if *dryRun {
		fmt.Fprintln(os.Stderr, "dry-run completed; no database changes were made")
	}
}
