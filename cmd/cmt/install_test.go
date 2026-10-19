//go:build linux || darwin

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease publishes vVERSION under dir the way GitHub serves release assets.
func fakeRelease(t *testing.T, dir, version string) (archive string) {
	t.Helper()
	name := fmt.Sprintf("cmt_%s_%s_%s", version, runtime.GOOS, runtime.GOARCH)
	stage := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(stage, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "cmt"), []byte("#!/bin/sh\necho 'cmt "+version+" (fake)'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "cmt.1"), []byte(".TH CMT 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	downloads := filepath.Join(dir, "download", "v"+version)
	if err := os.MkdirAll(downloads, 0755); err != nil {
		t.Fatal(err)
	}
	archive = filepath.Join(downloads, name+".tar.gz")
	if out, err := exec.Command("tar", "-czf", archive, "-C", filepath.Dir(stage), name).CombinedOutput(); err != nil {
		t.Fatalf("tar: %v: %s", err, out)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x  %s.tar.gz\n", sha256.Sum256(data), name)
	if err := os.WriteFile(archive+".sha256", []byte(sum), 0644); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestInstallScript(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl unavailable")
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		t.Skip("Intel Macs have no release yet")
	}
	releases, prefix := t.TempDir(), t.TempDir()
	run := func(args ...string) (string, error) {
		cmd := exec.Command("sh", append([]string{"../../install.sh", "--prefix", prefix}, args...)...)
		cmd.Env = append(os.Environ(), "CMT_RELEASES_URL=file://"+releases, "HOME="+t.TempDir())
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	install := func(version string) (string, error) { return run("--version", version) }
	installed := func() string {
		out, _ := exec.Command(filepath.Join(prefix, "bin", "cmt")).Output()
		return string(out)
	}

	fakeRelease(t, releases, "1.2.3")
	if out, err := install("1.2.3"); err != nil || !strings.Contains(out, "installed") || installed() != "cmt 1.2.3 (fake)\n" {
		t.Fatalf("install: %v: %s / %q", err, out, installed())
	}
	if _, err := os.Stat(filepath.Join(prefix, "share", "man", "man1", "cmt.1")); err != nil {
		t.Fatalf("man page: %v", err)
	}

	fakeRelease(t, releases, "1.3.0")
	if out, err := install("v1.3.0"); err != nil || !strings.Contains(out, "from 1.2.3 to 1.3.0") || installed() != "cmt 1.3.0 (fake)\n" {
		t.Fatalf("update: %v: %s / %q", err, out, installed())
	}

	// A tampered archive must leave the installed version untouched.
	archive := fakeRelease(t, releases, "1.4.0")
	if err := os.WriteFile(archive, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := install("1.4.0"); err == nil || !strings.Contains(out, "checksum mismatch") || installed() != "cmt 1.3.0 (fake)\n" {
		t.Fatalf("tampered archive: %v: %s / %q", err, out, installed())
	}
	if out, err := install("2.0.0"); err == nil || !strings.Contains(out, "download failed") {
		t.Fatalf("missing release: %v: %s", err, out)
	}
	for _, version := range []string{"1.0.0;rm", "--uninstall"} {
		if out, err := install(version); err == nil || !strings.Contains(out, "invalid version") {
			t.Fatalf("version validation %q: %v: %s", version, err, out)
		}
	}

	if out, err := run("--uninstall"); err != nil || !strings.Contains(out, "cmt remove") {
		t.Fatalf("uninstall: %v: %s", err, out)
	}
	for _, path := range []string{filepath.Join(prefix, "bin", "cmt"), filepath.Join(prefix, "share", "man", "man1", "cmt.1")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s survived uninstall: %v", path, err)
		}
	}
	if out, err := run("--uninstall"); err != nil || !strings.Contains(out, "is not installed") {
		t.Fatalf("second uninstall: %v: %s", err, out)
	}
}
