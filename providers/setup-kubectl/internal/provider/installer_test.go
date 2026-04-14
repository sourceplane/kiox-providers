package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseVersionSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		stable    bool
		exact     string
		major     string
		minor     string
		wantError bool
	}{
		{name: "latest", input: "latest", stable: true},
		{name: "stable", input: "stable", stable: true},
		{name: "minor", input: "1.30", major: "1", minor: "30"},
		{name: "minor with v", input: "v1.31", major: "1", minor: "31"},
		{name: "exact", input: "1.30.6", exact: "v1.30.6", major: "1", minor: "30"},
		{name: "invalid", input: "main", wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			spec, err := parseVersionSpec(test.input)
			if test.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersionSpec() error = %v", err)
			}
			if spec.stable != test.stable {
				t.Fatalf("stable = %t, want %t", spec.stable, test.stable)
			}
			if spec.exact != test.exact {
				t.Fatalf("exact = %q, want %q", spec.exact, test.exact)
			}
			if spec.major != test.major || spec.minor != test.minor {
				t.Fatalf("minor version = %s.%s, want %s.%s", spec.major, spec.minor, test.major, test.minor)
			}
		})
	}
}

func TestResolveTargetPathAddsWindowsExtension(t *testing.T) {
	t.Parallel()

	path, err := resolveTargetPath("", `C:\tools`, "kubectl", "windows")
	if err != nil {
		t.Fatalf("resolveTargetPath() error = %v", err)
	}

	want := filepath.Join(`C:\tools`, "bin", "kubectl.exe")
	if path != want {
		t.Fatalf("target path = %q, want %q", path, want)
	}
}

func TestInstallUsesCachedArtifactsForExactVersion(t *testing.T) {
	goos, goarch, err := kubectlPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("unsupported runtime for test: %v", err)
	}

	version := "v1.30.6"
	binary := []byte("kubectl-test-binary")
	digest := sha256.Sum256(binary)
	checksum := hex.EncodeToString(digest[:])

	var requests atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		switch request.URL.Path {
		case "/release/" + version + "/bin/" + goos + "/" + goarch + "/" + executableName(goos):
			_, _ = writer.Write(binary)
		case "/release/" + version + "/bin/" + goos + "/" + goarch + "/" + executableName(goos) + ".sha256":
			_, _ = writer.Write([]byte(checksum))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	firstInstallDir := t.TempDir()
	installer := NewInstaller()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	firstResult, err := installer.Install(ctx, Config{
		RequestedVersion: version,
		InstallDir:       firstInstallDir,
		CacheDir:         cacheDir,
		Mirrors:          []string{server.URL + "/release"},
		HTTPClient:       server.Client(),
	})
	if err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if firstResult.ResolvedVersion != version {
		t.Fatalf("resolved version = %q, want %q", firstResult.ResolvedVersion, version)
	}

	secondInstallDir := t.TempDir()
	secondResult, err := installer.Install(ctx, Config{
		RequestedVersion: version,
		InstallDir:       secondInstallDir,
		CacheDir:         cacheDir,
		Mirrors:          []string{"https://127.0.0.1:1/release"},
	})
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if !secondResult.UsedCache {
		t.Fatal("expected second install to use cache")
	}
	if requests.Load() != 2 {
		t.Fatalf("network requests = %d, want 2", requests.Load())
	}
	if ok, err := fileMatchesChecksum(secondResult.BinaryPath, checksum); err != nil || !ok {
		t.Fatalf("installed binary checksum mismatch, ok=%t err=%v", ok, err)
	}
}
