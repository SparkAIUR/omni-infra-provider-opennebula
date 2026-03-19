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
	machineRequest.Metadata().Labels().Set(omnires.LabelCluster, "HPLSVC")
	machineRequest.Metadata().Labels().Set(omnires.LabelControlPlaneRole, "")
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

		machineRequest := infrares.NewMachineRequest(id)
		machineRequest.TypedSpec().Value.ProviderData = `
schemaVersion: v1alpha2
flavor: small
networks:
  - name: prod-lan
`
		machineRequest.TypedSpec().Value.TalosVersion = "v1.9.0"
		machineRequest.Metadata().Labels().Set(omnires.LabelCluster, "HPLSVC")
		machineRequest.Metadata().Labels().Set(omnires.LabelWorkerRole, "")
		machineRequest.Metadata().Labels().Set(omnires.LabelMachineSet, machineSet)
		if err := state.Create(ctx, machineRequest); err != nil {
			t.Fatalf("create machine request %q: %v", id, err)
		}

		machine := resources.NewMachine("default", machineRequest.Metadata().ID())

		return machineRequest, machine, newProvisionContext(machineRequest, machine, "schem-123")
	}

	request1, machine1, pctx1 := makeWorker("worker-01", "workers-a")
	request2, machine2, pctx2 := makeWorker("worker-02", "workers-b")
	request3, machine3, pctx3 := makeWorker("worker-03", "workers-a")

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

	request4, machine4, pctx4 := makeWorker("worker-04", "workers-c")
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

func newAllocatorTestState(ctx context.Context, t *testing.T) runtimestate.State {
	t.Helper()

	_ = ctx

	return runtimestate.WrapCore(namespaced.NewState(inmem.Build))
}
