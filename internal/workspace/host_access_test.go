package workspace

import "testing"

func TestWorkspaceHostAccessContainerMounts(t *testing.T) {
	t.Parallel()

	access := WorkspaceHostAccess{
		Backend:     "container",
		TrustedRoot: "/tmp/trusted",
		ApprovedPaths: []WorkspaceApprovedHostPath{
			{Source: "/tmp/project-a", Status: workspaceApprovedPathStatusApproved},
			{Source: "/tmp/trusted", Status: workspaceApprovedPathStatusApproved},
			{Source: "/tmp/project-b", Status: "pending"},
		},
	}

	mounts := access.ContainerMounts()
	if len(mounts) != 2 {
		t.Fatalf("mount count = %d, want 2", len(mounts))
	}
	if mounts[0].Source != "/tmp/trusted" || mounts[0].Destination != "/mounts/host-0" {
		t.Fatalf("mount[0] = %+v", mounts[0])
	}
	if mounts[1].Source != "/tmp/project-a" || mounts[1].Destination != "/mounts/host-1" {
		t.Fatalf("mount[1] = %+v", mounts[1])
	}
	if mounts[0].Type != "bind" || mounts[1].Type != "bind" {
		t.Fatalf("unexpected mount type: %+v", mounts)
	}
}

func TestWorkspaceHostAccessContainerMountsSkipLocalBackend(t *testing.T) {
	t.Parallel()

	access := WorkspaceHostAccess{
		Backend:       "local",
		WorkspaceRoot: "/tmp/local-workspace",
		ApprovedPaths: []WorkspaceApprovedHostPath{{Source: "/tmp/project-a", Status: workspaceApprovedPathStatusApproved}},
	}

	if mounts := access.ContainerMounts(); len(mounts) != 0 {
		t.Fatalf("expected no mounts for local backend, got %+v", mounts)
	}
}
