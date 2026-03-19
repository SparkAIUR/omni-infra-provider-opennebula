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

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

// Deprovision implements infra.Provisioner.
func (p *Provisioner) Deprovision(ctx context.Context, logger *zap.Logger, machine *resources.Machine, _ *infrares.MachineRequest) error {
	vmID := GetVMID(machine)
	if vmID == 0 {
		logger.Info("vm id is not set, nothing to delete")
		return nil
	}

	if err := p.client.TerminateVM(ctx, vmID, p.config.Features.HardDelete); err != nil {
		if isNotFoundError(err) {
			logger.Info("vm already deleted", zap.Int("vm_id", vmID))
			clearProvisionedState(machine)
			SetPhase(machine, "deleted")
			return nil
		}

		return provision.NewRetryErrorf(10*time.Second, "terminate vm %d: %w", vmID, err)
	}

	clearProvisionedState(machine)
	SetPhase(machine, "deleted")
	logger.Info("terminated opennebula vm", zap.Int("vm_id", vmID))

	return nil
}

func clearProvisionedState(machine *resources.Machine) {
	SetLastError(machine, "")
	SetVMID(machine, 0)
	SetTemplateName(machine, "")
	SetTemplateID(machine, 0)
	SetImageName(machine, "")
	SetDatastore(machine, "")
	SetFlavor(machine, "")
	SetNetworkNames(machine, nil)
	machine.TypedSpec().Value.SchematicId = ""
	machine.TypedSpec().Value.TalosVersion = ""
	machine.TypedSpec().Value.VmName = ""
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return err == opennebula.ErrNotFound || strings.Contains(strings.ToLower(err.Error()), "not found")
}
