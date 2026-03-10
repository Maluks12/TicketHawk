package services

import (
	//"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

func TestDebugEnvLoading(t *testing.T) {
	dir, _ := os.Getwd()
	t.Logf("Working directory: %s", dir)

	// Walk up looking for go.mod
	search := dir
	for {
		gomod := filepath.Join(search, "go.mod")
		dotenv := filepath.Join(search, ".env")

		_, gomodErr := os.Stat(gomod)
		_, dotenvErr := os.Stat(dotenv)

		t.Logf("Checking dir: %s  |  go.mod exists: %v  |  .env exists: %v",
			search, gomodErr == nil, dotenvErr == nil)

		if gomodErr == nil {
			t.Logf("Found project root at: %s", search)
			if err := godotenv.Load(dotenv); err != nil {
				t.Logf(".env load error: %v", err)
			} else {
				t.Logf(".env loaded successfully")
			}
			break
		}

		parent := filepath.Dir(search)
		if parent == search {
			t.Log("Reached filesystem root without finding go.mod")
			break
		}
		search = parent
	}

	key := os.Getenv("SENDGRID_API_KEY")
	if key == "" {
		t.Log("SENDGRID_API_KEY: (empty)")
	} else {
		t.Logf("SENDGRID_API_KEY: %s...(len=%d)", key[:min(8, len(key))], len(key))
	}

	t.Logf("EMAIL_FROM_ADDRESS: %s", os.Getenv("EMAIL_FROM_ADDRESS"))
}