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
//	    Name:          "worker-01",
//	    ExtraTemplate: render,
//	})
type Client interface {
	LookupTemplateByName(context.Context, string) (TemplateRef, error)
	LookupImageByName(context.Context, string) (ImageRef, error)
	LookupDatastoreByName(context.Context, string) (DatastoreRef, error)
	LookupNetworksByName(context.Context, []string) ([]NetworkRef, error)
	InstantiateTemplate(context.Context, InstantiateRequest) (VMRef, error)
	GetVM(context.Context, int) (VMInfo, error)
	TerminateVM(context.Context, int, bool) error
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

// DatastoreRef is a resolved datastore reference.
type DatastoreRef struct {
	ID   int
	Name string
}

// NetworkRef is a resolved network reference.
type NetworkRef struct {
	ID   int
	Name string
}

// InstantiateRequest is the rendered VM instantiate input.
type InstantiateRequest struct {
	TemplateID    int
	VMName        string
	ExtraTemplate string
	Pending       bool
	CloneTemplate bool
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
