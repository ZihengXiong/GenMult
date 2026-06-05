package toolapproval

import (
	"testing"

	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/workspace"
)

func TestNeedsApprovalFileBypass(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true

	if needsApproval(cfg, "write", map[string]any{"path": "/data/tmp/output.txt"}) {
		t.Fatal("expected /data path to bypass write approval")
	}
	if needsApproval(cfg, "write", map[string]any{"path": "daily.md"}) {
		t.Fatal("expected relative /data path to bypass write approval")
	}
	if needsApproval(cfg, "edit", map[string]any{"path": "/tmp/output.txt"}) {
		t.Fatal("expected /tmp path to bypass edit approval")
	}
	if !needsApproval(cfg, "edit", map[string]any{"path": "/etc/passwd"}) {
		t.Fatal("expected non-bypassed edit path to require approval")
	}
}

func TestNeedsApprovalForceReviewOverridesBypass(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true
	cfg.Write.ForceReviewGlobs = []string{"/data/secret/**"}

	if !needsApproval(cfg, "write", map[string]any{"path": "/data/secret/token.txt"}) {
		t.Fatal("expected force-review path to require approval even under /data")
	}
}

func TestNeedsApprovalExecDefaultsToAllowed(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true

	if needsApproval(cfg, "exec", map[string]any{"command": "npm test"}) {
		t.Fatal("expected exec to be allowed by default")
	}
	if needsApproval(cfg, "exec", map[string]any{"command": "npm test && rm -rf /data"}) {
		t.Fatal("expected compound exec to be allowed when approval is disabled")
	}
}

func TestNeedsApprovalExecForceReview(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true
	cfg.Exec.ForceReviewCommands = []string{"rm"}

	if !needsApproval(cfg, "exec", map[string]any{"command": "rm file.txt"}) {
		t.Fatal("expected force-review command to require approval")
	}
}

func TestNeedsLocalHostPathApproval(t *testing.T) {
	access := workspace.WorkspaceHostAccess{
		Backend:       "local",
		WorkspaceRoot: "/Users/demo/workspace",
		ApprovedPaths: []workspace.WorkspaceApprovedHostPath{
			{Source: "/Users/demo/shared", Status: "approved"},
		},
	}

	if needsLocalHostPathApproval(access, "write", map[string]any{"path": "/Users/demo/workspace/note.txt"}) {
		t.Fatal("expected workspace-root write to bypass local host access approval")
	}
	if needsLocalHostPathApproval(access, "read", map[string]any{"path": "/Users/demo/shared/readme.md"}) {
		t.Fatal("expected approved host path to bypass local host access approval")
	}
	if !needsLocalHostPathApproval(access, "edit", map[string]any{"path": "/Users/demo/private/secret.txt"}) {
		t.Fatal("expected unapproved host path to require approval")
	}
	if !needsLocalHostPathApproval(access, "list", map[string]any{"path": "/Users/demo/private"}) {
		t.Fatal("expected unapproved directory listing to require approval")
	}
	if !needsLocalHostPathApproval(access, "exec", map[string]any{"command": "ls", "work_dir": "/Users/demo/private"}) {
		t.Fatal("expected exec work_dir outside whitelist to require approval")
	}
}

func TestHostAccessApprovalPath(t *testing.T) {
	if got := hostAccessApprovalPath("write", map[string]any{"path": "/Users/demo/project/file.txt"}); got != "/Users/demo/project" {
		t.Fatalf("write approval path = %q", got)
	}
	if got := hostAccessApprovalPath("list", map[string]any{"path": "/Users/demo/project/docs"}); got != "/Users/demo/project/docs" {
		t.Fatalf("list approval path = %q", got)
	}
	if got := hostAccessApprovalPath("exec", map[string]any{"work_dir": "/Users/demo/project"}); got != "/Users/demo/project" {
		t.Fatalf("exec approval path = %q", got)
	}
	if got := hostAccessApprovalPath("write", map[string]any{"path": "relative.txt"}); got != "" {
		t.Fatalf("relative path should not request host approval, got %q", got)
	}
}
