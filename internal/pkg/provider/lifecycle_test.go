package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/siderolabs/image-factory/pkg/schematic"
	"github.com/siderolabs/omni/client/pkg/infra/provision"
	infrares "github.com/siderolabs/omni/client/pkg/omni/resources/infra"
	omnires "github.com/siderolabs/omni/client/pkg/omni/resources/omni"
	"go.uber.org/zap"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	opennebulafake "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula/fake"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

func TestProvisionLifecycleWithFakeClient(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	client := opennebulafake.New()
	client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
	client.Images["talos-opennebula-amd64-v1.9.0-schematic-schem-123"] = opennebula.ImageRef{
		ID:        21,
		Name:      "talos-opennebula-amd64-v1.9.0-schematic-schem-123",
		Datastore: "fast-ssd",
	}
	client.Datastores["fast-ssd"] = opennebula.DatastoreRef{ID: 31, Name: "fast-ssd"}
	client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

	provisioner := NewProvisioner(client, cfg)
	machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
datastore: fast-ssd
networks:
  - name: prod-lan
`)
	state := resources.NewMachine("default", machineRequest.Metadata().ID())
	pctx := newProvisionContext(machineRequest, state, "schem-123")

	if err := provisioner.assignMachineUUID(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("assignMachineUUID() error = %v", err)
	}

	if err := provisioner.createSchematic(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("createSchematic() error = %v", err)
	}

	if err := provisioner.instantiateVM(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("instantiateVM() error = %v", err)
	}

	if err := provisioner.waitForVM(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("waitForVM() error = %v", err)
	}

	if GetVMID(state) == 0 {
		t.Fatal("expected vm id to be persisted")
	}

	if state.TypedSpec().Value.TemplateId != 11 {
		t.Fatalf("expected template id 11, got %d", state.TypedSpec().Value.TemplateId)
	}

	if got := client.LastInstantiate.ExtraTemplate; !strings.Contains(got, "USER_DATA_ENCODING = \"base64\"") {
		t.Fatalf("expected rendered USER_DATA_ENCODING, got %q", got)
	}

	if got, ok := pctx.MachineRequestStatus.Metadata().Labels().Get(omnires.LabelMachineInfraID); !ok || got == "" {
		t.Fatal("expected machine infra id label to be set")
	}

	if err := provisioner.Deprovision(context.Background(), zap.NewNop(), state, machineRequest); err != nil {
		t.Fatalf("Deprovision() error = %v", err)
	}

	if GetVMID(state) != 0 {
		t.Fatalf("expected vm id to be cleared, got %d", GetVMID(state))
	}

	if state.TypedSpec().Value.TemplateId != 0 || len(state.TypedSpec().Value.NetworkNames) != 0 {
		t.Fatalf("expected state to be cleared, got %+v", state.TypedSpec().Value)
	}
}

func TestInstantiateVMDuplicateRequestIsNoOp(t *testing.T) {
	t.Parallel()

	client := opennebulafake.New()
	provisioner := NewProvisioner(client, testConfig())
	machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
networks:
  - name: prod-lan
`)
	state := resources.NewMachine("default", machineRequest.Metadata().ID())
	SetVMID(state, 99)
	pctx := newProvisionContext(machineRequest, state, "schem-123")

	if err := provisioner.instantiateVM(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("instantiateVM() error = %v", err)
	}

	if client.LastInstantiate.TemplateID != 0 {
		t.Fatalf("expected instantiate to be skipped, got %+v", client.LastInstantiate)
	}
}

func TestInstantiateVMRetryableFailures(t *testing.T) {
	t.Parallel()

	t.Run("missing image", func(t *testing.T) {
		t.Parallel()

		client := opennebulafake.New()
		client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
		client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

		provisioner := NewProvisioner(client, testConfig())
		machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
networks:
  - name: prod-lan
`)
		state := resources.NewMachine("default", machineRequest.Metadata().ID())
		state.TypedSpec().Value.VmName = "worker-01"
		state.TypedSpec().Value.SchematicId = "schem-123"
		pctx := newProvisionContext(machineRequest, state, "schem-123")

		err := provisioner.instantiateVM(context.Background(), zap.NewNop(), pctx)
		if err == nil || !strings.Contains(err.Error(), "lookup image") {
			t.Fatalf("expected lookup image retry error, got %v", err)
		}
	})

	t.Run("instantiate failure", func(t *testing.T) {
		t.Parallel()

		client := opennebulafake.New()
		client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
		client.Images["talos-opennebula-amd64-v1.9.0-schematic-schem-123"] = opennebula.ImageRef{ID: 21, Name: "talos-opennebula-amd64-v1.9.0-schematic-schem-123"}
		client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}
		client.InstantiateErr = fmt.Errorf("temporary api failure")

		provisioner := NewProvisioner(client, testConfig())
		machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
networks:
  - name: prod-lan
`)
		state := resources.NewMachine("default", machineRequest.Metadata().ID())
		state.TypedSpec().Value.VmName = "worker-01"
		state.TypedSpec().Value.SchematicId = "schem-123"
		pctx := newProvisionContext(machineRequest, state, "schem-123")

		err := provisioner.instantiateVM(context.Background(), zap.NewNop(), pctx)
		if err == nil || !strings.Contains(err.Error(), "instantiate vm") {
			t.Fatalf("expected instantiate retry error, got %v", err)
		}
	})
}

func TestDeprovisionHandlesNotFoundAndHardDelete(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Features.HardDelete = true

	client := opennebulafake.New()
	provisioner := NewProvisioner(client, cfg)
	state := resources.NewMachine("default", "req-1")
	SetVMID(state, 77)
	SetTemplateID(state, 11)
	SetNetworkNames(state, []string{"prod-lan"})
	state.TypedSpec().Value.SchematicId = "schem-123"
	state.TypedSpec().Value.TalosVersion = "v1.9.0"

	if err := provisioner.Deprovision(context.Background(), zap.NewNop(), state, infrares.NewMachineRequest("req-1")); err != nil {
		t.Fatalf("Deprovision() not-found path error = %v", err)
	}

	if GetVMID(state) != 0 || state.TypedSpec().Value.SchematicId != "" {
		t.Fatalf("expected not-found path to clear state, got %+v", state.TypedSpec().Value)
	}

	client.VMs[88] = opennebula.VMInfo{ID: 88, Name: "vm-88", State: "ACTIVE", LCMState: "RUNNING"}
	SetVMID(state, 88)

	if err := provisioner.Deprovision(context.Background(), zap.NewNop(), state, infrares.NewMachineRequest("req-1")); err != nil {
		t.Fatalf("Deprovision() hard-delete path error = %v", err)
	}

	if !client.LastTerminateHard || client.LastTerminateID != 88 {
		t.Fatalf("expected hard delete for vm 88, got hard=%v id=%d", client.LastTerminateHard, client.LastTerminateID)
	}
}

type fakeFactory struct {
	schematicID string
}

func (f fakeFactory) EnsureSchematic(context.Context, schematic.Schematic) (string, error) {
	return f.schematicID, nil
}

func newMachineRequest(t *testing.T, providerData string) *infrares.MachineRequest {
	t.Helper()

	request := infrares.NewMachineRequest("request-01")
	request.TypedSpec().Value.ProviderData = providerData
	request.TypedSpec().Value.TalosVersion = "v1.9.0"

	return request
}

func newProvisionContext(machineRequest *infrares.MachineRequest, state *resources.Machine, schematicID string) provision.Context[*resources.Machine] {
	return provision.NewContext(
		machineRequest,
		infrares.NewMachineRequestStatus(machineRequest.Metadata().ID()),
		state,
		provision.ConnectionParams{JoinConfig: "cluster:\n  id: test"},
		fakeFactory{schematicID: schematicID},
		nil,
	)
}
