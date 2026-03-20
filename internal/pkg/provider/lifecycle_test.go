package provider

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/controller"
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
	client.Hypervisors = []string{"qemu"}
	client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
	client.Images["talos-opennebula-amd64-v1.9.0-schematic-schem-123"] = opennebula.ImageRef{
		ID:        21,
		Name:      "talos-opennebula-amd64-v1.9.0-schematic-schem-123",
		Datastore: "fast-ssd",
	}
	client.Datastores["fast-ssd"] = opennebula.DatastoreRef{ID: 31, Name: "fast-ssd"}
	client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

	provisioner := NewProvisioner(client, cfg, nil, nil)
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

	if got := client.LastInstantiate.ExtraTemplate; strings.Contains(got, "USER_DATA") {
		t.Fatalf("expected OpenNebula context without USER_DATA, got %q", got)
	} else if !strings.Contains(got, "SET_HOSTNAME = \"request-01\"") {
		t.Fatalf("expected rendered SET_HOSTNAME context, got %q", got)
	} else if !strings.Contains(got, "HYPERVISOR = \"qemu\"") {
		t.Fatalf("expected rendered qemu hypervisor, got %q", got)
	} else if !strings.Contains(got, "CPU_MODEL = [ MODEL = \"host-passthrough\" ]") {
		t.Fatalf("expected rendered host-passthrough cpu model, got %q", got)
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

func TestCreateSchematicPreservesConnectionKernelArgs(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	provisioner := NewProvisioner(opennebulafake.New(), cfg, nil, nil)
	machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
`)
	state := resources.NewMachine("default", machineRequest.Metadata().ID())
	factory := &fakeFactory{schematicID: "schem-123"}
	pctx := newProvisionContextWithFactory(machineRequest, state, factory)

	if err := provisioner.createSchematic(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("createSchematic() error = %v", err)
	}

	want := []string{
		"siderolink.api=https://omni.example.test:8090/?jointoken=test-token",
	}

	if got := factory.last.Customization.ExtraKernelArgs; !reflect.DeepEqual(got, want) {
		t.Fatalf("createSchematic() kernel args = %#v, want %#v", got, want)
	}
}

func TestInstantiateVMDuplicateRequestIsNoOp(t *testing.T) {
	t.Parallel()

	client := opennebulafake.New()
	provisioner := NewProvisioner(client, testConfig(), nil, nil)
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

func TestInstantiateVMAutoPrefersKVM(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.OpenNebula.Hypervisor = "auto"
	client := opennebulafake.New()
	client.Hypervisors = []string{"qemu", "kvm"}
	client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
	client.Images["talos-opennebula-amd64-v1.9.0-schematic-schem-123"] = opennebula.ImageRef{ID: 21, Name: "talos-opennebula-amd64-v1.9.0-schematic-schem-123"}
	client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

	provisioner := NewProvisioner(client, cfg, nil, nil)
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

	if err := provisioner.instantiateVM(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("instantiateVM() error = %v", err)
	}

	if !strings.Contains(client.LastInstantiate.ExtraTemplate, "HYPERVISOR = \"kvm\"") {
		t.Fatalf("expected rendered kvm hypervisor, got %q", client.LastInstantiate.ExtraTemplate)
	}
}

func TestInstantiateVMExplicitQEMU(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.OpenNebula.Hypervisor = "qemu"
	client := opennebulafake.New()
	client.Hypervisors = []string{"kvm"}
	client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
	client.Images["talos-opennebula-amd64-v1.9.0-schematic-schem-123"] = opennebula.ImageRef{ID: 21, Name: "talos-opennebula-amd64-v1.9.0-schematic-schem-123"}
	client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

	provisioner := NewProvisioner(client, cfg, nil, nil)
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

	if err := provisioner.instantiateVM(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("instantiateVM() error = %v", err)
	}

	if !strings.Contains(client.LastInstantiate.ExtraTemplate, "HYPERVISOR = \"qemu\"") {
		t.Fatalf("expected rendered qemu hypervisor, got %q", client.LastInstantiate.ExtraTemplate)
	}
}

func TestInstantiateVMAutoFailsWithoutSupportedHypervisor(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.OpenNebula.Hypervisor = "auto"
	client := opennebulafake.New()
	client.Hypervisors = nil
	client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
	client.Images["talos-opennebula-amd64-v1.9.0-schematic-schem-123"] = opennebula.ImageRef{ID: 21, Name: "talos-opennebula-amd64-v1.9.0-schematic-schem-123"}
	client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

	provisioner := NewProvisioner(client, cfg, nil, nil)
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
	if err == nil || !strings.Contains(err.Error(), "resolve hypervisor") {
		t.Fatalf("expected resolve hypervisor error, got %v", err)
	}
}

func TestInstantiateVMRetryableFailures(t *testing.T) {
	t.Parallel()

	t.Run("missing image", func(t *testing.T) {
		t.Parallel()

		client := opennebulafake.New()
		client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
		client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

		provisioner := NewProvisioner(client, testConfig(), nil, nil)
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
		if err == nil || !strings.Contains(err.Error(), "resolve image") {
			t.Fatalf("expected resolve image error, got %v", err)
		}
	})

	t.Run("instantiate failure", func(t *testing.T) {
		t.Parallel()

		client := opennebulafake.New()
		client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
		client.Images["talos-opennebula-amd64-v1.9.0-schematic-schem-123"] = opennebula.ImageRef{ID: 21, Name: "talos-opennebula-amd64-v1.9.0-schematic-schem-123"}
		client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}
		client.InstantiateErr = fmt.Errorf("temporary api failure")

		provisioner := NewProvisioner(client, testConfig(), nil, nil)
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

func TestWaitForVMRequiresRunningLCMState(t *testing.T) {
	t.Parallel()

	client := opennebulafake.New()
	provisioner := NewProvisioner(client, testConfig(), nil, nil)
	machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
networks:
  - name: prod-lan
`)
	state := resources.NewMachine("default", machineRequest.Metadata().ID())
	SetVMID(state, 101)
	client.VMs[101] = opennebula.VMInfo{
		ID:       101,
		Name:     "vm-101",
		State:    "ACTIVE",
		LCMState: "LCM_INIT",
	}
	pctx := newProvisionContext(machineRequest, state, "schem-123")

	err := provisioner.waitForVM(context.Background(), zap.NewNop(), pctx)
	if err == nil {
		t.Fatal("expected retry while vm is active but not running")
	}

	var retryErr *controller.RequeueError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected retry error, got %T: %v", err, err)
	}
}

func TestWaitForVMFailsOnBootFailure(t *testing.T) {
	t.Parallel()

	client := opennebulafake.New()
	provisioner := NewProvisioner(client, testConfig(), nil, nil)
	machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
networks:
  - name: prod-lan
`)
	state := resources.NewMachine("default", machineRequest.Metadata().ID())
	SetVMID(state, 102)
	client.VMs[102] = opennebula.VMInfo{
		ID:       102,
		Name:     "vm-102",
		State:    "ACTIVE",
		LCMState: "BOOT_FAILURE",
	}
	pctx := newProvisionContext(machineRequest, state, "schem-123")

	err := provisioner.waitForVM(context.Background(), zap.NewNop(), pctx)
	if err == nil {
		t.Fatal("expected terminal error on boot failure")
	}

	if !errors.Is(err, opennebula.ErrTerminal) {
		t.Fatalf("expected terminal error, got %T: %v", err, err)
	}
}

func TestInstantiateVMImportsMissingImageWhenConfigured(t *testing.T) {
	t.Parallel()

	payload := []byte("talos-image-data")
	checksum := fmt.Sprintf("%x", sha256.Sum256(payload))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/disk.qcow2":
			_, _ = w.Write(payload)
		case "/disk.qcow2.sha256":
			_, _ = w.Write([]byte(checksum + "  disk.qcow2\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.ImageManagement.ImportOnMiss = true
	cfg.ImageManagement.RequireChecksum = true
	cfg.ImageManagement.ArtifactURLTemplate = server.URL + "/disk.qcow2"
	cfg.ImageManagement.ChecksumURLTemplate = server.URL + "/disk.qcow2.sha256"
	cfg.ImageManagement.ImportTimeout = time.Second
	cfg.ImageManagement.PollInterval = 10 * time.Millisecond

	client := opennebulafake.New()
	client.Templates["talos-base"] = opennebula.TemplateRef{ID: 11, Name: "talos-base"}
	client.Datastores["fast-ssd"] = opennebula.DatastoreRef{ID: 31, Name: "fast-ssd"}
	client.Networks["prod-lan"] = opennebula.NetworkRef{ID: 41, Name: "prod-lan"}

	provisioner := NewProvisioner(client, cfg, nil, nil)
	machineRequest := newMachineRequest(t, `
schemaVersion: v1alpha1
flavor: small
datastore: fast-ssd
networks:
  - name: prod-lan
`)
	state := resources.NewMachine("default", machineRequest.Metadata().ID())
	state.TypedSpec().Value.VmName = "worker-01"
	state.TypedSpec().Value.SchematicId = "schem-123"
	pctx := newProvisionContext(machineRequest, state, "schem-123")

	if err := provisioner.instantiateVM(context.Background(), zap.NewNop(), pctx); err != nil {
		t.Fatalf("instantiateVM() error = %v", err)
	}

	if client.LastCreateImage.Name == "" {
		t.Fatal("expected image import request to be issued")
	}

	if state.TypedSpec().Value.ImageId == 0 || state.TypedSpec().Value.ImageChecksum == "" || state.TypedSpec().Value.ImageSource == "" {
		t.Fatalf("expected image import state to be persisted, got %+v", state.TypedSpec().Value)
	}
}

func TestDeprovisionHandlesNotFoundAndHardDelete(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Features.HardDelete = true

	client := opennebulafake.New()
	provisioner := NewProvisioner(client, cfg, nil, nil)
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

func TestDeprovisionForceDeletesLingeringShutdownVM(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Features.HardDelete = true

	client := opennebulafake.New()
	client.VMs[91] = opennebula.VMInfo{ID: 91, Name: "vm-91", State: "ACTIVE", LCMState: "RUNNING"}
	client.TerminateLeavesVM = true

	provisioner := NewProvisioner(client, cfg, nil, nil)
	state := resources.NewMachine("default", "req-1")
	SetVMID(state, 91)
	SetTemplateID(state, 11)
	state.TypedSpec().Value.SchematicId = "schem-123"

	if err := provisioner.Deprovision(context.Background(), zap.NewNop(), state, infrares.NewMachineRequest("req-1")); err != nil {
		t.Fatalf("Deprovision() lingering shutdown path error = %v", err)
	}

	if client.LastTerminateID != 91 || !client.LastTerminateHard {
		t.Fatalf("expected terminate hard for vm 91, got hard=%v id=%d", client.LastTerminateHard, client.LastTerminateID)
	}

	if client.LastForceDeleteID != 91 {
		t.Fatalf("expected force delete for vm 91, got id=%d", client.LastForceDeleteID)
	}

	if GetVMID(state) != 0 || state.TypedSpec().Value.SchematicId != "" {
		t.Fatalf("expected state to be cleared after forced delete, got %+v", state.TypedSpec().Value)
	}
}

func TestDeprovisionKeepsStateWhileLingeringVMStillExists(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Features.HardDelete = true

	client := opennebulafake.New()
	client.VMs[92] = opennebula.VMInfo{ID: 92, Name: "vm-92", State: "ACTIVE", LCMState: "RUNNING"}
	client.TerminateLeavesVM = true
	client.ForceDeleteLeavesVM = true

	provisioner := NewProvisioner(client, cfg, nil, nil)
	state := resources.NewMachine("default", "req-2")
	SetVMID(state, 92)
	SetTemplateID(state, 11)
	state.TypedSpec().Value.SchematicId = "schem-456"

	err := provisioner.Deprovision(context.Background(), zap.NewNop(), state, infrares.NewMachineRequest("req-2"))
	if err == nil {
		t.Fatal("expected retry while vm still exists")
	}

	var retryErr *controller.RequeueError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected retry error, got %T: %v", err, err)
	}

	if GetVMID(state) != 92 || state.TypedSpec().Value.SchematicId != "schem-456" {
		t.Fatalf("expected state to be retained while delete is in progress, got %+v", state.TypedSpec().Value)
	}
}

func TestDeprovisionForceDeletesDoneVMLingeringInInventory(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Features.HardDelete = true

	client := opennebulafake.New()
	client.VMs[93] = opennebula.VMInfo{ID: 93, Name: "vm-93", State: "DONE", LCMState: "LCM_INIT"}

	provisioner := NewProvisioner(client, cfg, nil, nil)
	state := resources.NewMachine("default", "req-3")
	SetVMID(state, 93)
	SetTemplateID(state, 11)
	state.TypedSpec().Value.SchematicId = "schem-789"

	if err := provisioner.Deprovision(context.Background(), zap.NewNop(), state, infrares.NewMachineRequest("req-3")); err != nil {
		t.Fatalf("Deprovision() lingering done vm path error = %v", err)
	}

	if client.LastTerminateID != 0 {
		t.Fatalf("expected terminate to be skipped for done vm, got id=%d", client.LastTerminateID)
	}

	if client.LastForceDeleteID != 0 {
		t.Fatalf("expected force delete to be skipped for done vm, got id=%d", client.LastForceDeleteID)
	}

	if GetVMID(state) != 0 || state.TypedSpec().Value.SchematicId != "" {
		t.Fatalf("expected state to be cleared after force deleting done vm, got %+v", state.TypedSpec().Value)
	}
}

type fakeFactory struct {
	schematicID string
	last        schematic.Schematic
}

func (f *fakeFactory) EnsureSchematic(_ context.Context, got schematic.Schematic) (string, error) {
	f.last = got

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
	return newProvisionContextWithFactory(machineRequest, state, &fakeFactory{schematicID: schematicID})
}

func newProvisionContextWithFactory(machineRequest *infrares.MachineRequest, state *resources.Machine, factory *fakeFactory) provision.Context[*resources.Machine] {
	return provision.NewContext(
		machineRequest,
		infrares.NewMachineRequestStatus(machineRequest.Metadata().ID()),
		state,
		provision.ConnectionParams{
			JoinConfig: "cluster:\n  id: test",
			KernelArgs: []string{
				"siderolink.api=https://omni.example.test:8090/?jointoken=test-token",
			},
		},
		factory,
		nil,
	)
}
