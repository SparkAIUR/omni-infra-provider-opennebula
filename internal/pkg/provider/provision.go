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

		schematicID, err := pctx.GenerateSchematicID(ctx, logger)
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

		data, resolvedResources, err := p.resolveRequest(ctx, pctx)
		if err != nil {
			SetLastError(pctx.State, err.Error())
			return err
		}
		SetDeleteMode(pctx.State, data.Lifecycle.DeleteMode)

		schematicID := pctx.State.TypedSpec().Value.SchematicId
		imageName, err := p.renderImageName(pctx.GetTalosVersion(), schematicID)
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
		}
		if err != nil {
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
			SetPhase(pctx.State, "image_importing")
			return p.clientError("instantiateVM", "resolve image", err)
		}
		SetPhase(pctx.State, "image_ready")

		templateRef, err := p.client.LookupTemplateByName(ctx, data.TemplateName)
		if err != nil {
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
			return p.clientError("instantiateVM", "lookup template", err)
		}

		networks, err := p.client.LookupNetworksByName(ctx, networkNames(data.Networks))
		if err != nil {
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
			return p.clientError("instantiateVM", "lookup networks", err)
		}

		if data.Datastore != "" {
			if _, err = p.client.LookupDatastoreByName(ctx, data.Datastore); err != nil {
				SetLastError(pctx.State, err.Error())
				SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
				return p.clientError("instantiateVM", "lookup datastore", err)
			}
		}

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
			ImageName:       imageResult.Image.Name,
			Datastore:       data.Datastore,
			Resources:       resolvedResources,
			Networks:        renderedNetworks(data, networks),
			FirmwareMode:    data.Firmware.Mode,
			SecureBoot:      effectiveSecureBoot(&data),
			GraphicsEnabled: effectiveGraphicsEnabled(&data),
			ContextKV:       contextKV,
			Placement:       resolvedPlacement(data),
			AdditionalDisks: data.AdditionalDisks,
		})

		stepLogger := provisionLogger(logger, pctx.State, pctx.GetRequestID(),
			zap.String("template_name", templateRef.Name),
			zap.String("image_name", imageResult.Image.Name),
			zap.String("datastore", data.Datastore),
			zap.Strings("network_names", networkNames(data.Networks)),
		)
		stepLogger.Debug("opennebula extra template rendered", zap.String("template", RedactTemplateForLog(rendered)))

		vmRef, err := p.client.InstantiateTemplate(ctx, opennebula.InstantiateRequest{
			TemplateID:    templateRef.ID,
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
		SetTemplateName(pctx.State, templateRef.Name)
		SetTemplateID(pctx.State, templateRef.ID)
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
		if vmID == 0 {
			SetLastRetryClassification(pctx.State, string(opennebula.ErrorClassRetryable))
			return p.retryError("waitForVM", 10*time.Second, "vm id is not set yet")
		}

		vmInfo, err := p.client.GetVM(ctx, vmID)
		if err != nil {
			SetLastError(pctx.State, err.Error())
			SetLastRetryClassification(pctx.State, string(opennebula.ClassifyError(err)))
			return p.clientError("waitForVM", "get vm", err)
		}

		if strings.EqualFold(vmInfo.LCMState, "RUNNING") || strings.EqualFold(vmInfo.State, "ACTIVE") {
			SetPhase(pctx.State, "vm_running")
			SetLastError(pctx.State, "")
			SetLastRetryClassification(pctx.State, "")
			return nil
		}

		SetLastRetryClassification(pctx.State, string(opennebula.ErrorClassRetryable))
		return p.retryError("waitForVM", 10*time.Second, "vm %d not running yet (state=%s lcm=%s)", vmID, vmInfo.State, vmInfo.LCMState)
	})
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

func (p *Provisioner) renderImageName(talosVersion, schematicID string) (string, error) {
	tpl, err := template.New("image-name").Parse(p.config.OpenNebula.ImageNamePattern)
	if err != nil {
		return "", fmt.Errorf("parse imageNamePattern: %w", err)
	}

	var builder strings.Builder
	if err := tpl.Execute(&builder, map[string]string{
		"Arch":         "amd64",
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

func resolvedPlacement(data ProviderData) ResolvedPlacement {
	requirements := make([]string, 0, 2)
	if data.Placement.Host != "" {
		requirements = append(requirements, fmt.Sprintf(`NAME = "%s"`, data.Placement.Host))
	}
	if data.Placement.Cluster != "" {
		requirements = append(requirements, fmt.Sprintf(`CLUSTER = "%s"`, data.Placement.Cluster))
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
