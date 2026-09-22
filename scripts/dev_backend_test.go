package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevBackend(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, env string
		want      string
	}{
		{"defaults", "", "compose:garfex-backend\nmigrate:garfex-backend_default\napi:127.0.0.1:8090\n"},
		{"override", "COMPOSE_PROJECT_NAME=local-test\nGARFEX_API_LISTEN_ADDR=127.0.0.1:8190\n", "compose:local-test\nmigrate:local-test_default\napi:127.0.0.1:8190\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "events")
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			envFile := filepath.Join(dir, "settings")
			config := "POSTGRES_USER=bootstrap\nPOSTGRES_PASSWORD=sample\nPOSTGRES_DB=garfex\nGARFEX_ADMIN_PASSWORD=sample\nGARFEX_APP_PASSWORD=sample\nGARFEX_ADMIN_DSN=postgres://admin:sample@db/garfex\nGARFEX_API_DSN=postgres://app:sample@127.0.0.1/garfex\n" + tc.env
			if err := os.WriteFile(envFile, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			docker := `#!/bin/sh
case "$1" in
 compose)
  [ "$2" = --project-directory ] && [ "$3" = "$TEST_ROOT" ] &&
  [ "$4" = --env-file ] && [ "$5" = "$GARFEX_ENV_FILE" ] &&
  [ "$6" = -p ] && [ "$8" = up ] && [ "$9" = -d ] &&
  [ "${10}" = --wait ] && [ "${11}" = db ] || exit 11
  printf 'compose:%s\n' "$7" >> "$TEST_LOG" ;;
 run)
  [ "$2" = --rm ] && [ "$3" = --network ] &&
  [ "$5" = -v ] && [ "$7" = migrate/migrate:v4.18.2 ] &&
  [ "$8" = -path=/migrations ] && [ "$9" = -database ] &&
  [ "${10}" = "$GARFEX_ADMIN_DSN" ] && [ "${11}" = up ] || exit 12
  printf 'migrate:%s\n' "$4" >> "$TEST_LOG" ;;
 *) exit 13 ;;
esac
`
			goFake := `#!/bin/sh
[ "$1" = run ] && [ "$2" = ./cmd/api ] && [ -f ./go.mod ] &&
[ "$GOWORK" = off ] && [ "$GARFEX_API_DSN" = 'postgres://app:sample@127.0.0.1/garfex' ] &&
[ -z "${GARFEX_ADMIN_DSN:-}" ] || exit 14
printf 'api:%s\n' "$GARFEX_API_LISTEN_ADDR" >> "$TEST_LOG"
`
			goFake = strings.ReplaceAll(goFake, "$TEST_LOG", log)
			for name, body := range map[string]string{"docker": docker, "go": goFake} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", filepath.Join(root, "scripts/dev-backend.sh"))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "GARFEX_ENV_FILE="+envFile, "TEST_ROOT="+root, "TEST_LOG="+log, "COMPOSE_PROJECT_NAME=")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("startup failed: %v (output redacted)", err)
			}
			if len(output) != 0 {
				t.Fatal("unexpected startup output (redacted)")
			}
			events, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if string(events) != tc.want {
				t.Errorf("events: %q, want %q", events, tc.want)
			}
		})
	}
}

func TestDevBackendRequiresDSNs(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, content string }{
		{"missing admin", "GARFEX_API_DSN=sample\n"},
		{"missing api", "GARFEX_ADMIN_DSN=sample\n"},
		{"missing both", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "settings")
			if err := os.WriteFile(file, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", filepath.Join(root, "scripts/dev-backend.sh"))
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GARFEX_ENV_FILE=" + file}
			out, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "GARFEX_ADMIN_DSN and GARFEX_API_DSN are required") {
				t.Fatal("required DSN validation failed (output redacted)")
			}
		})
	}
}
