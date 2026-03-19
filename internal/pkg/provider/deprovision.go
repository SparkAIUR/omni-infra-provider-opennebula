// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"time"

	"github.com/siderolabs/omni/client/pkg/infra/provision"
	infrares "github.com/siderolabs/omni/client/pkg/omni/resources/infra"
	"go.uber.org/zap"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

// Deprovision implements infra.Provisioner.
func (p *Provisioner) Deprovision(ctx context.Context, logger *zap.Logger, machine *resources.Machine, _ *infrares.MachineRequest) error {
	start := time.Now()
	err := p.deprovision(ctx, logger, machine)
	p.observeDeprovision(err, time.Since(start))
	return err
}

func (p *Provisioner) deprovision(ctx context.Context, logger *zap.Logger, machine *resources.Machine) error {
	vmID := GetVMID(machine)
	if vmID == 0 {
		provisionLogger(logger, machine, machine.Metadata().ID()).Info("vm id is not set, nothing to delete")
		return nil
	}

	SetPhase(machine, "delete_requested")
	if err := p.client.TerminateVM(ctx, vmID, p.config.Features.HardDelete); err != nil {
		SetLastRetryClassification(machine, string(opennebula.ClassifyError(err)))
		if opennebula.IsNotFoundError(err) {
			provisionLogger(logger, machine, machine.Metadata().ID()).Info("vm already deleted", zap.Int("vm_id", vmID))
			clearProvisionedState(machine)
			SetPhase(machine, "delete_complete")
			return nil
		}

		class := opennebula.ClassifyError(err)
		if opennebula.IsRetryableClass(class) {
			return provision.NewRetryErrorf(10*time.Second, "terminate vm %d: %w", vmID, err)
		}

		return err
	}

	clearProvisionedState(machine)
	SetPhase(machine, "delete_complete")
	provisionLogger(logger, machine, machine.Metadata().ID()).Info("terminated opennebula vm", zap.Int("vm_id", vmID))

	return nil
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
	machine.TypedSpec().Value.LastSuccessfulPhaseAt = ""
	machine.TypedSpec().Value.SchematicId = ""
	machine.TypedSpec().Value.TalosVersion = ""
	machine.TypedSpec().Value.VmName = ""
}
