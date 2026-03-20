package provider

import (
	"context"
	"testing"

	runtimestate "github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	"github.com/cosi-project/runtime/pkg/state/impl/namespaced"
	"github.com/siderolabs/omni/client/pkg/infra/provision"
	infrares "github.com/siderolabs/omni/client/pkg/omni/resources/infra"
	omnires "github.com/siderolabs/omni/client/pkg/omni/resources/omni"
	"go.uber.org/zap"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	opennebulafake "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula/fake"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

func TestAssignMachineUUIDClusterRoleSequenceControlPlane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := newAllocatorTestState(ctx, t)
	cfg := testConfig()
	cfg.Defaults.HostnameStrategy = providerconfig.HostnameStrategyClusterRoleSequence
	provisioner := NewProvisioner(opennebulafake.New(), cfg, nil, state)

	machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha2
flavor: small
networks:
  - name: prod-lan
`)
	machineSet := omnires.NewMachineSet("hplsvc-control-planes")
	machineSet.Metadata().Labels().Set(omnires.LabelCluster, "HPLSVC")
	machineSet.Metadata().Labels().Set(omnires.LabelControlPlaneRole, "")
	if err := state.Create(ctx, machineSet); err != nil {
		t.Fatalf("create machine set: %v", err)
	}

	machineRequest.Metadata().Labels().Set(omnires.LabelMachineRequestSet, machineSet.Metadata().ID())
	if err := state.Create(ctx, machineRequest); err != nil {
		t.Fatalf("create machine request: %v", err)
	}

	machine := resources.NewMachine("default", machineRequest.Metadata().ID())
	pctx := newProvisionContext(machineRequest, machine, "schem-123")

	if err := provisioner.assignMachineUUID(ctx, zap.NewNop(), pctx); err != nil {
		t.Fatalf("assignMachineUUID() error = %v", err)
	}

	if got := machine.TypedSpec().Value.VmName; got != "hplsvccp01" {
		t.Fatalf("expected control-plane vm name hplsvccp01, got %q", got)
	}

	if got := machine.TypedSpec().Value.ReservationId; got != "name-reservation/hplsvc/cp/1" {
		t.Fatalf("expected reservation id name-reservation/hplsvc/cp/1, got %q", got)
	}
}

func TestAssignMachineUUIDClusterRoleSequenceWorkerGapReuseAcrossMachineSets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := newAllocatorTestState(ctx, t)
	cfg := testConfig()
	cfg.Defaults.HostnameStrategy = providerconfig.HostnameStrategyClusterRoleSequence
	provisioner := NewProvisioner(opennebulafake.New(), cfg, nil, state)

	makeWorker := func(id, machineSet string) (*infrares.MachineRequest, *resources.Machine, provision.Context[*resources.Machine]) {
		t.Helper()

		set := omnires.NewMachineSet(machineSet)
		set.Metadata().Labels().Set(omnires.LabelCluster, "HPLSVC")
		set.Metadata().Labels().Set(omnires.LabelWorkerRole, "")
		if err := state.Create(ctx, set); err != nil && !runtimestate.IsConflictError(err) {
			t.Fatalf("create machine set %q: %v", machineSet, err)
		}

		machineRequest := infrares.NewMachineRequest(id)
		machineRequest.TypedSpec().Value.ProviderData = `
schemaVersion: v1alpha2
flavor: small
networks:
  - name: prod-lan
`
		machineRequest.TypedSpec().Value.TalosVersion = "v1.9.0"
		machineRequest.Metadata().Labels().Set(omnires.LabelMachineRequestSet, machineSet)
		if err := state.Create(ctx, machineRequest); err != nil {
			t.Fatalf("create machine request %q: %v", id, err)
		}

		machine := resources.NewMachine("default", machineRequest.Metadata().ID())

		return machineRequest, machine, newProvisionContext(machineRequest, machine, "schem-123")
	}

	request1, machine1, pctx1 := makeWorker("worker-01", "hplsvc-worker-a")
	request2, machine2, pctx2 := makeWorker("worker-02", "hplsvc-worker-b")
	request3, machine3, pctx3 := makeWorker("worker-03", "hplsvc-worker-a")

	for idx, item := range []struct {
		request *infrares.MachineRequest
		machine *resources.Machine
		pctx    provision.Context[*resources.Machine]
		want    string
	}{
		{request1, machine1, pctx1, "hplsvcw01"},
		{request2, machine2, pctx2, "hplsvcw02"},
		{request3, machine3, pctx3, "hplsvcw03"},
	} {
		if err := provisioner.assignMachineUUID(ctx, zap.NewNop(), item.pctx); err != nil {
			t.Fatalf("assignMachineUUID(%d) error = %v", idx, err)
		}

		if got := item.machine.TypedSpec().Value.VmName; got != item.want {
			t.Fatalf("expected vm name %q, got %q", item.want, got)
		}
	}

	if err := provisioner.releaseReservation(ctx, machine2); err != nil {
		t.Fatalf("releaseReservation() error = %v", err)
	}

	request4, machine4, pctx4 := makeWorker("worker-04", "hplsvc-worker-c")
	if err := provisioner.assignMachineUUID(ctx, zap.NewNop(), pctx4); err != nil {
		t.Fatalf("assignMachineUUID(reuse) error = %v", err)
	}

	if got := machine4.TypedSpec().Value.VmName; got != "hplsvcw02" {
		t.Fatalf("expected reused worker vm name hplsvcw02, got %q", got)
	}

	if request4.Metadata().ID() == request2.Metadata().ID() {
		t.Fatal("expected new machine request id for reused worker slot")
	}
}

func TestResolveClusterRoleInfersWorkerFromMachineRequestSetID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := newAllocatorTestState(ctx, t)
	provisioner := NewProvisioner(opennebulafake.New(), testConfig(), nil, state)

	machineRequest := infrares.NewMachineRequest("worker-01")
	machineRequest.Metadata().Labels().Set(omnires.LabelMachineRequestSet, "HPLSVC-worker-a")
	if err := state.Create(ctx, machineRequest); err != nil {
		t.Fatalf("create machine request: %v", err)
	}

	clusterName, role, err := provisioner.resolveClusterRole(ctx, machineRequest)
	if err != nil {
		t.Fatalf("resolveClusterRole() error = %v", err)
	}

	if clusterName != "HPLSVC" {
		t.Fatalf("expected cluster name HPLSVC, got %q", clusterName)
	}

	if role != nodeRoleWorker {
		t.Fatalf("expected worker role %q, got %q", nodeRoleWorker, role)
	}
}

func TestClusterRoleFromMachineRequestSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		machineSetID string
		clusterName  string
		role         string
	}{
		{
			name:         "control plane",
			machineSetID: "hplsvc-control-planes",
			clusterName:  "hplsvc",
			role:         nodeRoleControlPlane,
		},
		{
			name:         "worker",
			machineSetID: "hplsvc-workers-a",
			clusterName:  "hplsvc",
			role:         nodeRoleWorker,
		},
		{
			name:         "current worker naming",
			machineSetID: "hplsvc-worker-a",
			clusterName:  "hplsvc",
			role:         nodeRoleWorker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterName, role, err := ClusterRoleFromMachineRequestSet(tt.machineSetID)
			if err != nil {
				t.Fatalf("ClusterRoleFromMachineRequestSet() error = %v", err)
			}

			if clusterName != tt.clusterName {
				t.Fatalf("expected cluster %q, got %q", tt.clusterName, clusterName)
			}

			if role != tt.role {
				t.Fatalf("expected role %q, got %q", tt.role, role)
			}
		})
	}
}

func newAllocatorTestState(ctx context.Context, t *testing.T) runtimestate.State {
	t.Helper()

	_ = ctx

	return runtimestate.WrapCore(namespaced.NewState(inmem.Build))
}
