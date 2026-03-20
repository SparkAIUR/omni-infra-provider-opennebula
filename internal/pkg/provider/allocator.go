// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	cosistate "github.com/cosi-project/runtime/pkg/state"
	infrares "github.com/siderolabs/omni/client/pkg/omni/resources/infra"
	omnires "github.com/siderolabs/omni/client/pkg/omni/resources/omni"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

func (p *Provisioner) resolveVMName(
	ctx context.Context,
	machine *resources.Machine,
	requestID string,
	machineUUID string,
) (string, error) {
	if p.config.Defaults.HostnameStrategy != providerconfig.HostnameStrategyClusterRoleSequence {
		return CanonicalVMName(requestID), nil
	}

	if p.omniState == nil {
		return "", fmt.Errorf("hostnameStrategy %q requires omni state access", providerconfig.HostnameStrategyClusterRoleSequence)
	}

	if existing, ok, err := p.loadExistingReservation(ctx, machine, machineUUID); err != nil {
		return "", err
	} else if ok {
		return existing, nil
	}

	machineRequest, err := safe.StateGetByID[*infrares.MachineRequest](ctx, p.omniState, requestID)
	if err != nil {
		return "", fmt.Errorf("get machine request %q: %w", requestID, err)
	}

	clusterName, role, err := p.resolveClusterRole(ctx, machineRequest)
	if err != nil {
		return "", err
	}

	clusterPrefix, err := NormalizeClusterPrefix(clusterName)
	if err != nil {
		return "", err
	}

	for attempt := 0; attempt < 32; attempt++ {
		reservations, err := safe.StateListAll[*resources.NameReservation](ctx, p.omniState)
		if err != nil {
			return "", fmt.Errorf("list name reservations: %w", err)
		}

		ordinal := nextAvailableOrdinal(reservations, clusterPrefix, role)
		reservationID := reservationResourceID(clusterPrefix, role, ordinal)
		vmName := SequenceVMName(clusterPrefix, role, ordinal)

		reservation := resources.NewNameReservation(
			resources.NewNameReservation("", "").ResourceDefinition().DefaultNamespace,
			reservationID,
		)
		reservation.Metadata().Labels().Set(omnires.LabelCluster, clusterName)
		reservation.Metadata().Labels().Set(omnires.LabelMachineRequest, requestID)
		reservation.Metadata().Labels().Set(omnires.LabelHostname, vmName)
		reservation.TypedSpec().Value.ClusterName = clusterName
		reservation.TypedSpec().Value.ClusterPrefix = clusterPrefix
		reservation.TypedSpec().Value.Role = role
		reservation.TypedSpec().Value.Ordinal = int32(ordinal)
		reservation.TypedSpec().Value.VmName = vmName
		reservation.TypedSpec().Value.MachineRequestId = requestID
		reservation.TypedSpec().Value.MachineUuid = machineUUID
		reservation.TypedSpec().Value.StateResourceId = machine.Metadata().ID()
		reservation.TypedSpec().Value.CreatedAt = time.Now().UTC().Format(time.RFC3339)

		if err := p.omniState.Create(ctx, reservation); err != nil {
			if cosistate.IsConflictError(err) {
				continue
			}

			return "", fmt.Errorf("create name reservation %q: %w", reservationID, err)
		}

		SetClusterName(machine, clusterName)
		SetClusterPrefix(machine, clusterPrefix)
		SetNodeRole(machine, role)
		SetSequenceNumber(machine, ordinal)
		SetReservationID(machine, reservationID)

		return vmName, nil
	}

	return "", fmt.Errorf("allocate name reservation for %s/%s: too many conflicts", clusterPrefix, role)
}

func (p *Provisioner) resolveClusterRole(ctx context.Context, machineRequest *infrares.MachineRequest) (string, string, error) {
	clusterName, clusterOK := machineRequest.Metadata().Labels().Get(omnires.LabelCluster)
	role, roleErr := MachineRequestRole(machineRequest)
	if clusterOK && roleErr == nil {
		return clusterName, role, nil
	}

	machineSetID, ok := machineRequest.Metadata().Labels().Get(omnires.LabelMachineRequestSet)
	if !ok {
		if !clusterOK {
			return "", "", fmt.Errorf("machine request %q is missing %q label", machineRequest.Metadata().ID(), omnires.LabelCluster)
		}

		return "", "", roleErr
	}

	if !clusterOK || roleErr != nil {
		inferredClusterName, inferredRole, inferErr := ClusterRoleFromMachineRequestSet(machineSetID)
		if inferErr == nil {
			if !clusterOK {
				clusterName = inferredClusterName
				clusterOK = true
			}

			if roleErr != nil {
				role = inferredRole
				roleErr = nil
			}
		}
	}

	if clusterOK && roleErr == nil {
		return clusterName, role, nil
	}

	machineSet, err := safe.StateGetByID[*omnires.MachineSet](ctx, p.omniState, machineSetID)
	if err == nil {
		if !clusterOK {
			clusterName, clusterOK = machineSet.Metadata().Labels().Get(omnires.LabelCluster)
		}

		if roleErr != nil {
			role, roleErr = roleFromLabels(machineSet.Metadata().Labels(), machineSet.Metadata().ID())
		}
	}

	if !clusterOK {
		if err != nil {
			return "", "", fmt.Errorf("get machine set %q for machine request %q: %w", machineSetID, machineRequest.Metadata().ID(), err)
		}

		return "", "", fmt.Errorf("machine request %q and machine set %q are missing %q label", machineRequest.Metadata().ID(), machineSetID, omnires.LabelCluster)
	}

	if roleErr != nil {
		if err != nil {
			return "", "", fmt.Errorf("get machine set %q for machine request %q: %w", machineSetID, machineRequest.Metadata().ID(), err)
		}

		return "", "", roleErr
	}

	return clusterName, role, nil
}

func (p *Provisioner) loadExistingReservation(ctx context.Context, machine *resources.Machine, machineUUID string) (string, bool, error) {
	if p.omniState == nil {
		return "", false, nil
	}

	if reservationID := GetReservationID(machine); reservationID != "" {
		reservation, err := safe.StateGetByID[*resources.NameReservation](ctx, p.omniState, reservationID)
		if err == nil {
			p.applyReservation(machine, reservation)
			return reservation.TypedSpec().Value.VmName, true, nil
		}

		if !cosistate.IsNotFoundError(err) {
			return "", false, fmt.Errorf("get name reservation %q: %w", reservationID, err)
		}

		if machine.TypedSpec().Value.ClusterPrefix != "" && machine.TypedSpec().Value.NodeRole != "" && machine.TypedSpec().Value.SequenceNumber > 0 {
			vmName, err := p.recreateReservation(ctx, machine, machineUUID)
			if err != nil {
				return "", false, err
			}

			return vmName, true, nil
		}
	}

	if machineUUID == "" {
		return "", false, nil
	}

	reservations, err := safe.StateListAll[*resources.NameReservation](ctx, p.omniState)
	if err != nil {
		return "", false, fmt.Errorf("list name reservations: %w", err)
	}

	for reservation := range reservations.All() {
		if reservation.TypedSpec().Value.MachineUuid != machineUUID {
			continue
		}

		p.applyReservation(machine, reservation)

		return reservation.TypedSpec().Value.VmName, true, nil
	}

	if machine.TypedSpec().Value.VmName != "" && machine.TypedSpec().Value.ClusterPrefix != "" && machine.TypedSpec().Value.NodeRole != "" && machine.TypedSpec().Value.SequenceNumber > 0 {
		vmName, err := p.recreateReservation(ctx, machine, machineUUID)
		if err != nil {
			return "", false, err
		}

		return vmName, true, nil
	}

	return "", false, nil
}

func (p *Provisioner) recreateReservation(ctx context.Context, machine *resources.Machine, machineUUID string) (string, error) {
	reservationID := reservationResourceID(
		machine.TypedSpec().Value.ClusterPrefix,
		machine.TypedSpec().Value.NodeRole,
		int(machine.TypedSpec().Value.SequenceNumber),
	)
	reservation := resources.NewNameReservation(
		resources.NewNameReservation("", "").ResourceDefinition().DefaultNamespace,
		reservationID,
	)
	reservation.TypedSpec().Value.ClusterName = machine.TypedSpec().Value.ClusterName
	reservation.TypedSpec().Value.ClusterPrefix = machine.TypedSpec().Value.ClusterPrefix
	reservation.TypedSpec().Value.Role = machine.TypedSpec().Value.NodeRole
	reservation.TypedSpec().Value.Ordinal = machine.TypedSpec().Value.SequenceNumber
	reservation.TypedSpec().Value.VmName = machine.TypedSpec().Value.VmName
	reservation.TypedSpec().Value.MachineRequestId = machine.Metadata().ID()
	reservation.TypedSpec().Value.MachineUuid = machineUUID
	reservation.TypedSpec().Value.StateResourceId = machine.Metadata().ID()
	reservation.TypedSpec().Value.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	reservation.Metadata().Labels().Set(omnires.LabelCluster, machine.TypedSpec().Value.ClusterName)
	reservation.Metadata().Labels().Set(omnires.LabelMachineRequest, machine.Metadata().ID())
	reservation.Metadata().Labels().Set(omnires.LabelHostname, machine.TypedSpec().Value.VmName)

	if err := p.omniState.Create(ctx, reservation); err != nil && !cosistate.IsConflictError(err) {
		return "", fmt.Errorf("recreate name reservation %q: %w", reservationID, err)
	}

	SetReservationID(machine, reservationID)

	return machine.TypedSpec().Value.VmName, nil
}

func (p *Provisioner) releaseReservation(ctx context.Context, machine *resources.Machine) error {
	if p.omniState == nil {
		return nil
	}

	reservationID := GetReservationID(machine)
	if reservationID == "" && GetMachineUUID(machine) != "" {
		reservations, err := safe.StateListAll[*resources.NameReservation](ctx, p.omniState)
		if err != nil {
			return fmt.Errorf("list name reservations: %w", err)
		}

		for reservation := range reservations.All() {
			if reservation.TypedSpec().Value.MachineUuid == GetMachineUUID(machine) {
				reservationID = reservation.Metadata().ID()
				break
			}
		}
	}

	if reservationID == "" {
		return nil
	}

	ptr := resource.NewMetadata(
		resources.NewNameReservation("", "").ResourceDefinition().DefaultNamespace,
		resources.NewNameReservation("", "").ResourceDefinition().Type,
		reservationID,
		resource.VersionUndefined,
	)

	if err := p.omniState.Destroy(ctx, ptr); err != nil && !cosistate.IsNotFoundError(err) {
		return fmt.Errorf("destroy name reservation %q: %w", reservationID, err)
	}

	return nil
}

func (p *Provisioner) applyReservation(machine *resources.Machine, reservation *resources.NameReservation) {
	SetClusterName(machine, reservation.TypedSpec().Value.ClusterName)
	SetClusterPrefix(machine, reservation.TypedSpec().Value.ClusterPrefix)
	SetNodeRole(machine, reservation.TypedSpec().Value.Role)
	SetSequenceNumber(machine, int(reservation.TypedSpec().Value.Ordinal))
	SetReservationID(machine, reservation.Metadata().ID())
	machine.TypedSpec().Value.VmName = reservation.TypedSpec().Value.VmName
}

func nextAvailableOrdinal(reservations safe.List[*resources.NameReservation], clusterPrefix, role string) int {
	used := map[int]struct{}{}

	for reservation := range reservations.All() {
		if reservation.TypedSpec().Value.ClusterPrefix != clusterPrefix || reservation.TypedSpec().Value.Role != role {
			continue
		}

		if reservation.TypedSpec().Value.Ordinal > 0 {
			used[int(reservation.TypedSpec().Value.Ordinal)] = struct{}{}
		}
	}

	for ordinal := 1; ; ordinal++ {
		if _, ok := used[ordinal]; !ok {
			return ordinal
		}
	}
}

func reservationResourceID(clusterPrefix, role string, ordinal int) string {
	return fmt.Sprintf("name-reservation/%s/%s/%d", clusterPrefix, role, ordinal)
}
