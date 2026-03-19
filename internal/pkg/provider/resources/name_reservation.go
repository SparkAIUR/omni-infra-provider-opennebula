// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package resources

import (
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/meta"
	"github.com/cosi-project/runtime/pkg/resource/protobuf"
	"github.com/cosi-project/runtime/pkg/resource/typed"
	"github.com/siderolabs/omni/client/pkg/infra"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/api/specs"
	providermeta "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/meta"
)

// NewNameReservation creates new NameReservation.
func NewNameReservation(ns, id string) *NameReservation {
	return typed.NewResource[NameReservationSpec, NameReservationExtension](
		resource.NewMetadata(ns, infra.ResourceType("NameReservation", providermeta.ProviderID), id, resource.VersionUndefined),
		protobuf.NewResourceSpec(&specs.NameReservationSpec{}),
	)
}

// NameReservation describes provider-owned cluster-role sequence reservations.
type NameReservation = typed.Resource[NameReservationSpec, NameReservationExtension]

// NameReservationSpec wraps specs.NameReservationSpec.
type NameReservationSpec = protobuf.ResourceSpec[specs.NameReservationSpec, *specs.NameReservationSpec]

// NameReservationExtension provides auxiliary methods for NameReservation.
type NameReservationExtension struct{}

// ResourceDefinition implements [typed.Extension] interface.
func (NameReservationExtension) ResourceDefinition() meta.ResourceDefinitionSpec {
	return meta.ResourceDefinitionSpec{
		Type:             infra.ResourceType("NameReservation", providermeta.ProviderID),
		Aliases:          []resource.Type{},
		DefaultNamespace: infra.ResourceNamespace(providermeta.ProviderID),
		PrintColumns:     []meta.PrintColumn{},
	}
}
