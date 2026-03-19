// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"strings"
	"time"

	"github.com/siderolabs/omni/client/pkg/infra/provision"
	infrares "github.com/siderolabs/omni/client/pkg/omni/resources/infra"
	"go.uber.org/zap"
	"go.yaml.in/yaml/v4"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

// Deprovision implements infra.Provisioner.
func (p *Provisioner) Deprovision(ctx context.Context, logger *zap.Logger, machine *resources.Machine, machineRequest *infrares.MachineRequest) error {
	start := time.Now()
	err := p.deprovision(ctx, logger, machine, machineRequest)
	p.observeDeprovision(err, time.Since(start))
	return err
}

func (p *Provisioner) deprovision(ctx context.Context, logger *zap.Logger, machine *resources.Machine, machineRequest *infrares.MachineRequest) error {
	vmID := GetVMID(machine)
	if vmID == 0 {
		if err := p.releaseReservation(ctx, machine); err != nil {
			return provision.NewRetryErrorf(10*time.Second, "release name reservation: %w", err)
		}

		clearProvisionedState(machine)
		provisionLogger(logger, machine, machine.Metadata().ID()).Info("vm id is not set, nothing to delete")
		return nil
	}

	deleteHard := p.resolveDeleteMode(machine, machineRequest)

	SetPhase(machine, "delete_requested")
	deleted, err := p.ensureVMDeleted(ctx, logger, machine, vmID, deleteHard)
	if err != nil {
		return err
	}

	if !deleted {
		return provision.NewRetryErrorf(10*time.Second, "wait for vm %d deletion", vmID)
	}

	if err := p.releaseReservation(ctx, machine); err != nil {
		return provision.NewRetryErrorf(10*time.Second, "release name reservation: %w", err)
	}

	clearProvisionedState(machine)
	SetPhase(machine, "delete_complete")
	provisionLogger(logger, machine, machine.Metadata().ID()).Info("terminated opennebula vm", zap.Int("vm_id", vmID))

	return nil
}

func (p *Provisioner) ensureVMDeleted(ctx context.Context, logger *zap.Logger, machine *resources.Machine, vmID int, deleteHard bool) (bool, error) {
	info, err := p.client.GetVM(ctx, vmID)
	if err != nil {
		SetLastRetryClassification(machine, string(opennebula.ClassifyError(err)))
		if opennebula.IsNotFoundError(err) {
			provisionLogger(logger, machine, machine.Metadata().ID()).Info("vm already deleted", zap.Int("vm_id", vmID))
			return true, nil
		}

		class := opennebula.ClassifyError(err)
		if opennebula.IsRetryableClass(class) {
			return false, provision.NewRetryErrorf(10*time.Second, "get vm %d: %w", vmID, err)
		}

		return false, err
	}

	if vmIsTerminallyDeleted(info) {
		return true, nil
	}

	if vmRequiresForceDelete(info, deleteHard) {
		if err := p.forceDeleteVM(ctx, machine, vmID); err != nil {
			return false, err
		}
	} else {
		if err := p.terminateVM(ctx, machine, vmID, deleteHard); err != nil {
			return false, err
		}
	}

	info, err = p.client.GetVM(ctx, vmID)
	if err != nil {
		SetLastRetryClassification(machine, string(opennebula.ClassifyError(err)))
		if opennebula.IsNotFoundError(err) {
			return true, nil
		}

		class := opennebula.ClassifyError(err)
		if opennebula.IsRetryableClass(class) {
			return false, provision.NewRetryErrorf(10*time.Second, "confirm vm %d deletion: %w", vmID, err)
		}

		return false, err
	}

	if vmIsTerminallyDeleted(info) {
		return true, nil
	}

	if vmRequiresForceDelete(info, deleteHard) {
		if err := p.forceDeleteVM(ctx, machine, vmID); err != nil {
			return false, err
		}

		info, err = p.client.GetVM(ctx, vmID)
		if err != nil {
			SetLastRetryClassification(machine, string(opennebula.ClassifyError(err)))
			if opennebula.IsNotFoundError(err) {
				return true, nil
			}

			class := opennebula.ClassifyError(err)
			if opennebula.IsRetryableClass(class) {
				return false, provision.NewRetryErrorf(10*time.Second, "confirm forced vm %d deletion: %w", vmID, err)
			}

			return false, err
		}
	}

	provisionLogger(logger, machine, machine.Metadata().ID()).Info(
		"vm deletion still in progress",
		zap.Int("vm_id", vmID),
		zap.String("vm_state", info.State),
		zap.String("vm_lcm_state", info.LCMState),
	)

	return false, nil
}

func (p *Provisioner) terminateVM(ctx context.Context, machine *resources.Machine, vmID int, deleteHard bool) error {
	if err := p.client.TerminateVM(ctx, vmID, deleteHard); err != nil {
		SetLastRetryClassification(machine, string(opennebula.ClassifyError(err)))
		if opennebula.IsNotFoundError(err) {
			return nil
		}

		class := opennebula.ClassifyError(err)
		if opennebula.IsRetryableClass(class) {
			return provision.NewRetryErrorf(10*time.Second, "terminate vm %d: %w", vmID, err)
		}

		return err
	}

	return nil
}

func (p *Provisioner) forceDeleteVM(ctx context.Context, machine *resources.Machine, vmID int) error {
	if err := p.client.ForceDeleteVM(ctx, vmID); err != nil {
		SetLastRetryClassification(machine, string(opennebula.ClassifyError(err)))
		if opennebula.IsNotFoundError(err) {
			return nil
		}

		class := opennebula.ClassifyError(err)
		if opennebula.IsRetryableClass(class) {
			return provision.NewRetryErrorf(10*time.Second, "force delete vm %d: %w", vmID, err)
		}

		return err
	}

	return nil
}

func vmRequiresForceDelete(info opennebula.VMInfo, deleteHard bool) bool {
	if !deleteHard {
		return false
	}

	switch normalizeState(info.LCMState) {
	case "SHUTDOWN", "SHUTDOWN_POWEROFF", "SHUTDOWN_UNDEPLOY", "EPILOG", "EPILOG_STOP", "EPILOG_UNDEPLOY", "CLEANUP_DELETE":
		return true
	default:
		return false
	}
}

func vmIsTerminallyDeleted(info opennebula.VMInfo) bool {
	return normalizeState(info.State) == "DONE"
}

func normalizeState(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func (p *Provisioner) resolveDeleteMode(machine *resources.Machine, machineRequest *infrares.MachineRequest) bool {
	if machine != nil {
		if deleteMode := GetDeleteMode(machine); deleteMode != "" {
			return deleteMode == "hard"
		}
	}

	if machineRequest == nil || strings.TrimSpace(machineRequest.TypedSpec().Value.ProviderData) == "" {
		return p.config.Features.HardDelete
	}

	var data ProviderData
	if err := yaml.Unmarshal([]byte(machineRequest.TypedSpec().Value.ProviderData), &data); err != nil {
		return p.config.Features.HardDelete
	}

	if data.Lifecycle.DeleteMode == "hard" && p.config.Features.HardDelete {
		return true
	}

	if data.Lifecycle.DeleteMode == "terminate" || data.Lifecycle.DeleteMode == "normal" {
		return false
	}

	return p.config.Features.HardDelete
}

func clearProvisionedState(machine *resources.Machine) {
	SetLastError(machine, "")
	SetLastRetryClassification(machine, "")
	SetVMID(machine, 0)
	SetTemplateName(machine, "")
	SetTemplateID(machine, 0)
	SetImageID(machine, 0)
	SetImageName(machine, "")
	SetImageSource(machine, "")
	SetImageChecksum(machine, "")
	SetDatastore(machine, "")
	SetFlavor(machine, "")
	SetNetworkNames(machine, nil)
	SetDeleteMode(machine, "")
	machine.TypedSpec().Value.LastSuccessfulPhaseAt = ""
	machine.TypedSpec().Value.SchematicId = ""
	machine.TypedSpec().Value.TalosVersion = ""
	machine.TypedSpec().Value.VmName = ""
	machine.TypedSpec().Value.ClusterName = ""
	machine.TypedSpec().Value.ClusterPrefix = ""
	machine.TypedSpec().Value.NodeRole = ""
	machine.TypedSpec().Value.SequenceNumber = 0
	machine.TypedSpec().Value.ReservationId = ""
}
