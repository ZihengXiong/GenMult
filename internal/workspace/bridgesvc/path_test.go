package bridgesvc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/ZihengXiong/GenMult/internal/workspace/bridgepb"
)

func TestLocalPathResolverMapsDataMountToWorkspaceRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	srv := New(Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		DataMount:         "/data",
		AllowHostAbsolute: true,
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    "/data/notes/demo.txt",
		Content: []byte("demo"),
	}); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "notes", "demo.txt")) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read mapped file failed: %v", err)
	}
	if string(got) != "demo" {
		t.Fatalf("mapped file = %q, want demo", string(got))
	}
}

func TestLocalPathResolverAllowsHostAbsolutePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	srv := New(Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		DataMount:         "/data",
		AllowHostAbsolute: true,
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    outside,
		Content: []byte("outside"),
	}); err != nil {
		t.Fatalf("WriteFile absolute path failed: %v", err)
	}
	got, err := os.ReadFile(outside) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read absolute file failed: %v", err)
	}
	if string(got) != "outside" {
		t.Fatalf("absolute file = %q, want outside", string(got))
	}
}

func TestLocalPathResolverAllowsWhitelistedHostAbsolutePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	approvedRoot := t.TempDir()
	approvedPath := filepath.Join(approvedRoot, "approved.txt")
	srv := New(Options{
		DefaultWorkDir:   root,
		WorkspaceRoot:    root,
		DataMount:        "/data",
		AllowedHostPaths: []string{approvedRoot},
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    approvedPath,
		Content: []byte("approved"),
	}); err != nil {
		t.Fatalf("WriteFile approved absolute path failed: %v", err)
	}
	got, err := os.ReadFile(approvedPath) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read approved absolute file failed: %v", err)
	}
	if string(got) != "approved" {
		t.Fatalf("approved absolute file = %q, want approved", string(got))
	}
}

func TestLocalPathResolverRedirectsDisallowedAbsolutePathIntoWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outsideRoot := t.TempDir()
	outsidePath := filepath.Join(outsideRoot, "outside.txt")
	srv := New(Options{
		DefaultWorkDir: root,
		WorkspaceRoot:  root,
		DataMount:      "/data",
	})

	if _, err := srv.WriteFile(context.Background(), &pb.WriteFileRequest{
		Path:    outsidePath,
		Content: []byte("workspace"),
	}); err != nil {
		t.Fatalf("WriteFile disallowed absolute path failed: %v", err)
	}
	if _, err := os.Stat(outsidePath); !os.IsNotExist(err) {
		t.Fatalf("expected outside path to remain untouched, stat err=%v", err)
	}

	trimmed := strings.TrimPrefix(filepath.Clean(outsidePath), string(filepath.Separator))
	workspacePath := filepath.Join(root, trimmed)
	got, err := os.ReadFile(workspacePath) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read redirected workspace file failed: %v", err)
	}
	if string(got) != "workspace" {
		t.Fatalf("redirected file = %q, want workspace", string(got))
	}
}
