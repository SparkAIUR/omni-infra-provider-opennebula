// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package provider implements the OpenNebula Omni infrastructure provider core.
package provider

import (
	"context"
	"fmt"
	"net"
	"strings"
	"text/template"
	"time"

	cosistate "github.com/cosi-project/runtime/pkg/state"
	"github.com/google/uuid"
	"github.com/siderolabs/omni/client/pkg/infra/provision"
	"go.uber.org/zap"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/imagemanager"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/observability"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

// Provisioner orchestrates Omni requests against OpenNebula.
type Provisioner struct {
	client       opennebula.Client
	config       providerconfig.Config
	metrics      *observability.Metrics
	imageManager *imagemanager.Manager
	omniState    cosistate.State
}

// NewProvisioner creates a new OpenNebula provisioner.
func NewProvisioner(client opennebula.Client, cfg providerconfig.Config, metrics *observability.Metrics, omniState cosistate.State) *Provisioner {
	return &Provisioner{
		client:       client,
		config:       cfg,
		metrics:      metrics,
		imageManager: imagemanager.New(client, cfg, metrics),
		omniState:    omniState,
	}
}

// ProvisionSteps implements infra.Provisioner.
func (p *Provisioner) ProvisionSteps() []provision.Step[*resources.Machine] {
	return []provision.Step[*resources.Machine]{
		provision.NewStep("assignMachineUUID", p.assignMachineUUID),
		provision.NewStep("createSchematic", p.createSchematic),
		provision.NewStep("instantiateVM", p.instantiateVM),
		provision.NewStep("waitForVM", p.waitForVM),
	}
}

func (p *Provisioner) assignMachineUUID(ctx context.Context, logger *zap.Logger, pctx provision.Context[*resources.Machine]) error {
	return p.runProvisionStep("assignMachineUUID", func() error {
		uuidValue := GetMachineUUID(pctx.State)
		if uuidValue == "" {
			uuidValue = uuid.NewString()
			SetMachineUUID(pctx.State, uuidValue)
		}

		vmName, err := p.resolveVMName(ctx, pctx.State, pctx.GetRequestID(), uuidValue)
		if err != nil {
			return err
		}

		pctx.State.TypedSpec().Value.TalosVersion = pctx.GetTalosVersion()
		pctx.State.TypedSpec().Value.VmName = vmName
		pctx.SetMachineUUID(uuidValue)
		SetPhase(pctx.State, "machine_uuid_assigned")
		provisionLogger(logger, pctx.State, pctx.GetRequestID()).Info("assigned machine identity")

		return nil
	})
}

func (p *Provisioner) createSchematic(ctx context.Context, logger *zap.Logger, pctx provision.Context[*resources.Machine]) error {
	return p.runProvisionStep("createSchematic", func() error {
		if pctx.State.TypedSpec().Value.SchematicId != "" {
			return nil
		}

		schematicID, err := pctx.GenerateSchematicID(
			ctx,
			logger,
			provision.WithoutConnectionParams(),
			provision.WithExtraKernelArgs(pctx.ConnectionParams.KernelArgs...),
		)
		if err != nil {
			SetLastRetryClassification(pctx.State, string(opennebula.ErrorClassRetryable))
			return p.retryError("createSchematic", 10*time.Second, "generate schematic: %w", err)
		}

		pctx.State.TypedSpec().Value.SchematicId = schematicID
		SetPhase(pctx.State, "schematic_ready")
		provisionLogger(logger, pctx.State, pctx.GetRequestID()).Info("resolved schematic")

		return nil
	})
}

func (p *Provisioner) createHostnamePatch(ctx context.Context, _ *zap.Logger, pctx provision.Context[*resources.Machine]) error {
	return p.runProvisionStep("createHostnamePatch", func() error {
		hostname := pctx.State.TypedSpec().Value.VmName
		patchName := hostname + "-opennebula-hostname"

		if err := pctx.CreateConfigPatch(ctx, patchName, HostnameConfigPatch(hostname)); err != nil {
			SetLastRetryClassification(pctx.State, string(opennebula.ErrorClassRetryable))
			return p.retryError("createHostnamePatch", 10*time.Second, "create hostname patch: %w", err)
		}

		SetPhase(pctx.State, "hostname_patch_ready")

		return nil
	})
}

func (p *Provisioner) instantiateVM(ctx context.Context, logger *zap.Logger, pctx provision.Context[*resources.Machine]) error {
	return p.runProvisionStep("instantiateVM", func() error {
		if GetVMID(pctx.State) != 0 {
			return nil
		}

		plan, err := p.resolveProvisionPlan(ctx, pctx)
		if err != nil {
			SetPreflight(pctx.State, string(PreflightStatusFail), []string{err.Error()}, nil)
			SetLastError(pctx.State, err.Error())
			return err
		}
		data := plan.Data
		resolvedResources := plan.Resources
		SetDeleteMode(pctx.State, data.Lifecycle.DeleteMode)
		SetResolvedHypervisor(pctx.State, plan.Hypervisor)
		SetResolvedHost(pctx.State, plan.Placement.Selected.ID, plan.Placement.Selected.Name)
		SetResolvedHostTags(pctx.State, plan.Placement.Selected.Tags)
		SetResolvedCluster(pctx.State, plan.Placement.Selected.ClusterID, plan.Placement.Selected.ClusterName)
		SetResolvedStorageProfile(pctx.State, plan.StorageProfile)
		SetResolvedDatastoreCapabilities(pctx.State, plan.Datastore.Capabilities)
		SetPlacementDecision(pctx.State, plan.Placement.Reason, plan.Placement.ScoreSummary)
		SetPreflight(pctx.State, string(plan.Preflight.Status), plan.Preflight.Errors, plan.Preflight.Warnings)
		SetBootstrapProfile(pctx.State, plan.BootstrapProfile)
		SetDatastoreID(pctx.State, plan.Datastore.ID)
		SetNetworkIDs(pctx.State, networkIDs(plan.Networks))
		SetDrift(pctx.State, DriftStatusHealthy, nil)
		SetDiagnosticFingerprint(pctx.State, "")

		schematicID := pctx.State.TypedSpec().Value.SchematicId
		imageName, err := p.renderImageName(pctx.GetTalosVersion(), schematicID, data.Datastore)
		if err != nil {
			SetLastError(pctx.State, err.Error())
			return err
		}

		imageResult, err := p.imageManager.Resolve(ctx, imagemanager.ResolveRequest{
			ImageName:        imageName,
			Arch:             "amd64",
			TalosVersion:     pctx.GetTalosVersion(),
			SchematicID:      schematicID,
			Datastore:        data.Datastore,
			AllowImport:      imageImportPreference(data),
			ExistingImageID:  int(pctx.State.TypedSpec().Value.ImageId),
			ExistingChecksum: pctx.State.TypedSpec().Value.ImageChecksum,
			ExistingSource:   pctx.State.TypedSpec().Value.ImageSource,
			ProviderManaged:  pctx.State.TypedSpec().Value.ImageSource != "",
		})
		if imageResult.Image.ID != 0 {
			SetImageID(pctx.State, imageResult.Image.ID)
			SetImageName(pctx.State, imageResult.Image.Name)
			SetImageSource(pctx.State, imageResult.SourceURL)
			SetImageChecksum(pctx.State, imageResult.Checksum)
			SetImageAction(pctx.State, imageResult.Action, imageResult.CacheHit, imageResult.ChecksumVerified)
		}
		if err != nil {
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
			SetPhase(pctx.State, "image_importing")
			return p.clientError("instantiateVM", "resolve image", err)
		}
		SetPhase(pctx.State, "image_ready")

		hostname := pctx.State.TypedSpec().Value.VmName

		contextKV := map[string]string{
			"SET_HOSTNAME": hostname,
		}
		if data.NetworkContextMode == "manual" {
			contextKV["NETWORK"] = "NO"
			for index, nic := range data.StaticNetwork {
				prefix := fmt.Sprintf("ETH%d", index)
				if nic.IP != "" {
					contextKV[prefix+"_IP"] = nic.IP
					contextKV[prefix+"_METHOD"] = "static"
				}
				if nic.Mask != "" {
					contextKV[prefix+"_MASK"] = nic.Mask
				}
				if nic.Network != "" {
					contextKV[prefix+"_NETWORK"] = nic.Network
				}
				if nic.Gateway != "" {
					contextKV[prefix+"_GATEWAY"] = nic.Gateway
				}
				if len(nic.DNS) > 0 {
					contextKV[prefix+"_DNS"] = strings.Join(nic.DNS, " ")
				}
				if nic.MAC != "" {
					contextKV[prefix+"_MAC"] = nic.MAC
				}
				if nic.Name != "" {
					contextKV[prefix+"_NAME"] = nic.Name
				}
			}
		} else {
			contextKV["NETWORK"] = "YES"
		}

		rendered := RenderTemplate(RenderInput{
			VMName:          hostname,
			MachineUUID:     GetMachineUUID(pctx.State),
			Hypervisor:      plan.Hypervisor,
			ImageName:       imageResult.Image.Name,
			Datastore:       data.Datastore,
			Resources:       resolvedResources,
			Networks:        renderedNetworks(data, plan.Networks),
			FirmwareMode:    data.Firmware.Mode,
			SecureBoot:      effectiveSecureBoot(&data),
			GraphicsEnabled: effectiveGraphicsEnabled(&data),
			ContextKV:       contextKV,
			Placement:       resolvedPlacement(data, plan.Placement),
			AdditionalDisks: data.AdditionalDisks,
		})

		stepLogger := provisionLogger(logger, pctx.State, pctx.GetRequestID(),
			zap.String("template_name", plan.Template.Name),
			zap.String("image_name", imageResult.Image.Name),
			zap.String("datastore", data.Datastore),
			zap.String("resolved_hypervisor", plan.Hypervisor),
			zap.String("resolved_host", plan.Placement.Selected.Name),
			zap.Strings("network_names", networkNames(data.Networks)),
		)
		stepLogger.Debug("opennebula extra template rendered", zap.String("template", RedactTemplateForLog(rendered)))

		vmRef, err := p.client.InstantiateTemplate(ctx, opennebula.InstantiateRequest{
			TemplateID:    plan.Template.ID,
			VMName:        hostname,
			ExtraTemplate: rendered,
			Pending:       false,
			CloneTemplate: false,
		})
		if err != nil {
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
			return p.clientError("instantiateVM", "instantiate vm", err)
		}

		SetVMID(pctx.State, vmRef.ID)
		SetTemplateName(pctx.State, plan.Template.Name)
		SetTemplateID(pctx.State, plan.Template.ID)
		SetImageID(pctx.State, imageResult.Image.ID)
		SetImageName(pctx.State, imageResult.Image.Name)
		SetImageSource(pctx.State, imageResult.SourceURL)
		SetImageChecksum(pctx.State, imageResult.Checksum)
		SetDatastore(pctx.State, data.Datastore)
		SetFlavor(pctx.State, data.Flavor)
		SetNetworkNames(pctx.State, networkNames(data.Networks))
		SetPhase(pctx.State, "vm_instantiated")
		SetLastError(pctx.State, "")
		SetLastRetryClassification(pctx.State, "")
		pctx.SetMachineInfraID(fmt.Sprintf("%d", vmRef.ID))
		provisionLogger(logger, pctx.State, pctx.GetRequestID()).Info("instantiated opennebula vm")

		return nil
	})
}

func (p *Provisioner) waitForVM(ctx context.Context, _ *zap.Logger, pctx provision.Context[*resources.Machine]) error {
	return p.runProvisionStep("waitForVM", func() error {
		vmID := GetVMID(pctx.State)
		bootstrapProfile := pctx.State.TypedSpec().Value.BootstrapProfile
		retryInterval := 10 * time.Second
		if bootstrapProfile == providerconfig.BootstrapProfileLab {
			retryInterval = 15 * time.Second
		}

		if vmID == 0 {
			SetLastRetryClassification(pctx.State, string(opennebula.ErrorClassRetryable))
			if p.config.Diagnostics.BootstrapHints.Enabled {
				SetDiagnosticFingerprint(pctx.State, DiagnosticFingerprintBootstrapPending)
			}
			return p.retryError("waitForVM", retryInterval, "vm id is not set yet")
		}

		vmInfo, err := p.client.GetVM(ctx, vmID)
		if err != nil {
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
			if opennebula.IsNotFoundError(err) {
				SetDrift(pctx.State, DriftStatusActionable, []string{fmt.Sprintf("vm %d is missing in OpenNebula", vmID)})
			}
			if p.config.Diagnostics.BootstrapHints.Enabled {
				SetDiagnosticFingerprint(pctx.State, DiagnosticFingerprintBootstrapFailure)
			}
			return p.clientError("waitForVM", "get vm", err)
		}

		if vmIsRunning(vmInfo) {
			SetPhase(pctx.State, "vm_running")
			SetLastError(pctx.State, "")
			SetLastRetryClassification(pctx.State, "")
			return nil
		}

		if vmReachedFailureState(vmInfo) {
			err = fmt.Errorf("%w: vm %d reached non-running state=%s lcm=%s", opennebula.ErrTerminal, vmID, vmInfo.State, vmInfo.LCMState)
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ErrorClassTerminal))
			SetDrift(pctx.State, DriftStatusWarning, []string{fmt.Sprintf("vm %d entered failure state %s/%s", vmID, vmInfo.State, vmInfo.LCMState)})
			if p.config.Diagnostics.BootstrapHints.Enabled {
				SetDiagnosticFingerprint(pctx.State, fmt.Sprintf("%s:%s/%s:%s", DiagnosticFingerprintBootstrapFailure, vmInfo.State, vmInfo.LCMState, bootstrapProfile))
			}

			return err
		}

		SetLastRetryClassification(pctx.State, string(opennebula.ErrorClassRetryable))
		if p.config.Diagnostics.BootstrapHints.Enabled {
			SetDiagnosticFingerprint(pctx.State, fmt.Sprintf("%s:%s/%s:%s", DiagnosticFingerprintBootstrapPending, vmInfo.State, vmInfo.LCMState, bootstrapProfile))
		}
		return p.retryError("waitForVM", retryInterval, "vm %d not running yet (state=%s lcm=%s bootstrap_profile=%s)", vmID, vmInfo.State, vmInfo.LCMState, bootstrapProfile)
	})
}

func (p *Provisioner) resolveHypervisor(ctx context.Context) (string, error) {
	switch p.config.OpenNebula.Hypervisor {
	case "", providerconfig.HypervisorAuto:
		return p.client.ResolveHypervisor(ctx, opennebula.HypervisorResolveRequest{
			ResourcePool: p.config.OpenNebula.ResourcePool,
		})
	case providerconfig.HypervisorKVM, providerconfig.HypervisorQEMU:
		return p.config.OpenNebula.Hypervisor, nil
	default:
		return "", fmt.Errorf("%w: unsupported hypervisor %q", opennebula.ErrPolicy, p.config.OpenNebula.Hypervisor)
	}
}

func vmIsRunning(info opennebula.VMInfo) bool {
	return strings.EqualFold(normalizeState(info.State), "ACTIVE") &&
		strings.EqualFold(normalizeState(info.LCMState), "RUNNING")
}

func vmReachedFailureState(info opennebula.VMInfo) bool {
	state := normalizeState(info.State)
	lcmState := normalizeState(info.LCMState)

	if state == "DONE" || state == "FAILED" {
		return true
	}

	switch lcmState {
	case
		"UNKNOWN",
		"BOOT_FAILURE",
		"BOOT_MIGRATE_FAILURE",
		"PROLOG_FAILURE",
		"PROLOG_MIGRATE_FAILURE",
		"PROLOG_RESUME_FAILURE",
		"EPILOG_FAILURE",
		"EPILOG_STOP_FAILURE",
		"EPILOG_UNDEPLOY_FAILURE",
		"EPILOG_UNDEPLOY_FAILURE_STOP",
		"EPILOG_UNDEPLOY_FAILURE_DELETE",
		"EPILOG_DELETE_FAILURE",
		"HOTPLUG_FAILURE",
		"HOTPLUG_SNAPSHOT_FAILURE",
		"HOTPLUG_NIC_FAILURE",
		"HOTPLUG_SAVEAS_FAILURE",
		"HOTPLUG_SAVEAS_POWEROFF_FAILURE",
		"HOTPLUG_SAVEAS_SUSPENDED_FAILURE",
		"DISK_SNAPSHOT_FAILURE",
		"DISK_SNAPSHOT_DELETE_FAILURE",
		"DISK_SNAPSHOT_REVERT_FAILURE",
		"DISK_RESIZE_FAILURE",
		"DISK_ATTACH_FAILURE",
		"DISK_DETACH_FAILURE",
		"NIC_ATTACH_FAILURE",
		"NIC_DETACH_FAILURE",
		"DISK_SNAPSHOT_SUSPENDED_FAILURE",
		"DISK_SNAPSHOT_DELETE_POWEROFF",
		"DISK_SNAPSHOT_DELETE_SUSPENDED",
		"CLEANUP_RESUBMIT":
		return true
	default:
		return false
	}
}

func (p *Provisioner) resolveRequest(ctx context.Context, pctx provision.Context[*resources.Machine]) (ProviderData, ResolvedResources, error) {
	var data ProviderData

	if err := pctx.UnmarshalProviderData(&data); err != nil {
		return ProviderData{}, ResolvedResources{}, fmt.Errorf("unmarshal providerData: %w", err)
	}

	if err := ValidateProviderData(&data, p.config); err != nil {
		return ProviderData{}, ResolvedResources{}, err
	}

	resolved, err := ResolveResources(data, p.config)
	if err != nil {
		return ProviderData{}, ResolvedResources{}, fmt.Errorf("resolve resources: %w", err)
	}

	if data.Datastore == "" {
		data.Datastore = p.config.StoragePolicies.DefaultDatastore
	}

	if data.Datastore != "" {
		if _, err = p.client.LookupDatastoreByName(ctx, data.Datastore); err != nil {
			return ProviderData{}, ResolvedResources{}, err
		}
	}

	return data, resolved, nil
}

func (p *Provisioner) renderImageName(talosVersion, schematicID, datastore string) (string, error) {
	tpl, err := template.New("image-name").Parse(p.config.OpenNebula.ImageNamePattern)
	if err != nil {
		return "", fmt.Errorf("parse imageNamePattern: %w", err)
	}

	var builder strings.Builder
	if err := tpl.Execute(&builder, map[string]string{
		"Arch":         "amd64",
		"Datastore":    datastore,
		"TalosVersion": talosVersion,
		"SchematicID":  schematicID,
	}); err != nil {
		return "", fmt.Errorf("render imageNamePattern: %w", err)
	}

	return builder.String(), nil
}

func imageImportPreference(data ProviderData) *bool {
	switch data.ImagePolicy.Mode {
	case "reuse-only":
		return boolPtr(false)
	case "reuse-or-import":
		return boolPtr(true)
	default:
		return nil
	}
}

func renderedNetworks(data ProviderData, resolved []opennebula.NetworkRef) []RenderedNetwork {
	models := map[string]string{}
	for _, network := range data.Networks {
		models[network.Name] = network.Model
	}

	rendered := make([]RenderedNetwork, 0, len(resolved))
	for _, network := range resolved {
		rendered = append(rendered, RenderedNetwork{
			Name:  network.Name,
			Model: models[network.Name],
		})
	}

	return rendered
}

func networkIDs(networks []opennebula.NetworkRef) []int {
	ids := make([]int, 0, len(networks))
	for _, network := range networks {
		ids = append(ids, network.ID)
	}

	return ids
}

func resolvedPlacement(data ProviderData, placement PlacementDecision) ResolvedPlacement {
	requirements := make([]string, 0, 2)
	hostName := data.Placement.Host
	if placement.Selected.Name != "" {
		hostName = placement.Selected.Name
	}
	if hostName != "" {
		requirements = append(requirements, fmt.Sprintf(`NAME = "%s"`, hostName))
	}
	clusterName := data.Placement.Cluster
	if placement.Selected.ClusterName != "" {
		clusterName = placement.Selected.ClusterName
	}
	if clusterName != "" {
		requirements = append(requirements, fmt.Sprintf(`CLUSTER = "%s"`, clusterName))
	}

	return ResolvedPlacement{
		SchedRequirements: strings.Join(requirements, " & "),
		VMGroupName:       data.Placement.VMGroup,
		VMGroupRole:       data.Placement.Role,
	}
}

func deriveNetworkCIDR(ipAddress, mask string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(ipAddress)).To4()
	if ip == nil {
		return "", fmt.Errorf("invalid IPv4 address %q", ipAddress)
	}

	maskIP := net.ParseIP(strings.TrimSpace(mask)).To4()
	if maskIP == nil {
		return "", fmt.Errorf("invalid IPv4 mask %q", mask)
	}

	ipNet := net.IPNet{IP: ip.Mask(net.IPMask(maskIP)), Mask: net.IPMask(maskIP)}

	return ipNet.IP.String(), nil
}
