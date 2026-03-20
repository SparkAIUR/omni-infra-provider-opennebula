// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package opennebula wraps OpenNebula-specific API interactions.
package opennebula

import "context"

// Client abstracts OpenNebula operations required by the provider.
//
// Example:
//
//	vmRef, err := client.InstantiateTemplate(ctx, InstantiateRequest{
//	    TemplateID:    42,
//	    VMName:        "worker-01",
//	    ExtraTemplate: rendered,
//	})
type Client interface {
	LookupTemplateByName(context.Context, string) (TemplateRef, error)
	LookupImageByName(context.Context, string) (ImageRef, error)
	LookupDatastoreByName(context.Context, string) (DatastoreRef, error)
	LookupNetworksByName(context.Context, []string) ([]NetworkRef, error)
	ResolveHypervisor(context.Context, HypervisorResolveRequest) (string, error)
	CreateImage(context.Context, CreateImageRequest) (ImageRef, error)
	GetImage(context.Context, int) (ImageInfo, error)
	DeleteImage(context.Context, int) error
	InstantiateTemplate(context.Context, InstantiateRequest) (VMRef, error)
	GetVM(context.Context, int) (VMInfo, error)
	TerminateVM(context.Context, int, bool) error
	ForceDeleteVM(context.Context, int) error
}

// TemplateRef is a resolved VM template reference.
type TemplateRef struct {
	ID   int
	Name string
}

// ImageRef is a resolved image reference.
type ImageRef struct {
	ID        int
	Name      string
	Datastore string
	SizeMiB   int
}

// ImageInfo is the provider-facing image state.
type ImageInfo struct {
	ID        int
	Name      string
	Datastore string
	SizeMiB   int
	State     string
	Source    string
}

// DatastoreRef is a resolved datastore reference.
type DatastoreRef struct {
	ID     int
	Name   string
	FreeMB int
}

// NetworkRef is a resolved network reference.
type NetworkRef struct {
	ID   int
	Name string
}

// CreateImageRequest is the rendered input for importing a Talos image.
type CreateImageRequest struct {
	DatastoreID int
	Name        string
	SourcePath  string
	SourceURL   string
	Driver      string
	Format      string
}

// InstantiateRequest is the rendered VM instantiate input.
type InstantiateRequest struct {
	TemplateID    int
	VMName        string
	ExtraTemplate string
	Pending       bool
	CloneTemplate bool
}

// HypervisorResolveRequest scopes host-based hypervisor detection.
type HypervisorResolveRequest struct {
	ResourcePool string
}

// VMRef is a lightweight created VM reference.
type VMRef struct {
	ID   int
	Name string
}

// VMInfo is the provider-facing VM state.
type VMInfo struct {
	ID       int
	Name     string
	State    string
	LCMState string
}
