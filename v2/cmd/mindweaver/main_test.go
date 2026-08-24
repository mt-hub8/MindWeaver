package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

func TestConfigInitAndCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindweaver.json")
	var output bytes.Buffer
	if err := run(context.Background(), []string{"config", "init", "-file", path, "-vault", "./my-vault"}, &output); err != nil {
		t.Fatalf("config init: %v", err)
	}
	if !strings.Contains(output.String(), "schema v1") {
		t.Fatalf("init output = %q", output.String())
	}

	output.Reset()
	if err := run(context.Background(), []string{"config", "check", "-file", path}, &output); err != nil {
		t.Fatalf("config check: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "valid schema v1") || !strings.Contains(got, "vault=./my-vault") {
		t.Fatalf("check output = %q", got)
	}
}

func TestUnknownCommandIsInvalid(t *testing.T) {
	err := run(context.Background(), []string{"launch"}, &bytes.Buffer{})
	if !apperror.IsKind(err, apperror.KindInvalid) || exitCode(err) != 2 {
		t.Fatalf("run() error = %v, exit = %d", err, exitCode(err))
	}
}

func TestVersionIsRunnable(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"version"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "mindweaver 0.1.0-dev") {
		t.Fatalf("version output = %q", output.String())
	}
}

func TestServeProcessOwnsVaultServesHealthAndReopensAfterKill(t *testing.T) {
	if os.Getenv("MINDWEAVER_SERVE_HELPER") == "1" {
		t.Skip("parent-only test")
	}
	root := t.TempDir()
	configPath := filepath.Join(root, "mindweaver.v1.json")
	vaultPath := filepath.Join(root, "vault")

	first := startServeHelper(t, configPath, vaultPath)
	firstURL := waitForHelperPrefix(t, first, "http://")
	assertProcessHealth(t, firstURL)

	second := startServeHelper(t, configPath, vaultPath)
	locked := waitForHelperPrefix(t, second, "HELPER_ERROR:")
	if !strings.Contains(locked, "already open") {
		t.Fatalf("second process error = %q", locked)
	}
	waitHelper(t, second)

	killHelper(t, first)
	third := startServeHelper(t, configPath, vaultPath)
	thirdURL := waitForHelperPrefix(t, third, "http://")
	assertProcessHealth(t, thirdURL)
	killHelper(t, third)
}

func TestServeHelperProcess(t *testing.T) {
	if os.Getenv("MINDWEAVER_SERVE_HELPER") != "1" {
		return
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		fmt.Println("HELPER_ERROR:missing arguments")
		return
	}
	if err := run(context.Background(), os.Args[separator+1:], os.Stdout); err != nil {
		fmt.Printf("HELPER_ERROR:%s\n", apperror.PublicMessage(err))
	}
}

type serveHelper struct {
	command *exec.Cmd
	lines   <-chan string
	done    <-chan error
	stderr  *bytes.Buffer
}

func startServeHelper(t *testing.T, configPath, vaultPath string) serveHelper {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestServeHelperProcess$", "--", "serve", "-config", configPath, "-vault", vaultPath, "-no-browser")
	command.Env = append(os.Environ(), "MINDWEAVER_SERVE_HELPER=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	lines := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	go func() { done <- command.Wait(); close(done) }()
	return serveHelper{command: command, lines: lines, done: done, stderr: stderr}
}

func waitForHelperPrefix(t *testing.T, helper serveHelper, prefix string) string {
	t.Helper()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case line, open := <-helper.lines:
			if !open {
				t.Fatalf("helper exited before %q; stderr=%q", prefix, helper.stderr.String())
			}
			if strings.HasPrefix(line, prefix) {
				return line
			}
		case <-timeout.C:
			_ = helper.command.Process.Kill()
			t.Fatalf("timed out waiting for helper prefix %q; stderr=%q", prefix, helper.stderr.String())
		}
	}
}

func assertProcessHealth(t *testing.T, launchURL string) {
	t.Helper()
	origin := strings.SplitN(launchURL, "/#", 2)[0]
	request, err := http.NewRequest(http.MethodGet, origin+localhttp.HealthPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(localhttp.NonBrowserHeader, localhttp.NonBrowserReadOnly)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	client.CloseIdleConnections()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("process health status = %d", response.StatusCode)
	}
}

func killHelper(t *testing.T, helper serveHelper) {
	t.Helper()
	if err := helper.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-helper.done:
	case <-time.After(10 * time.Second):
		t.Fatal("killed helper did not exit")
	}
}

func waitHelper(t *testing.T, helper serveHelper) {
	t.Helper()
	select {
	case err := <-helper.done:
		if err != nil {
			t.Fatalf("helper exit = %v; stderr=%q", err, helper.stderr.String())
		}
	case <-time.After(10 * time.Second):
		_ = helper.command.Process.Kill()
		t.Fatal("helper did not exit")
	}
}
