// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package fake

import (
	"context"
	"fmt"
	"sync"

	parent "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
)

// Client is a simple fake adapter used by tests.
type Client struct {
	mu                sync.Mutex
	Templates         map[string]parent.TemplateRef
	Images            map[string]parent.ImageRef
	ImageInfoByID     map[int]parent.ImageInfo
	Datastores        map[string]parent.DatastoreRef
	Networks          map[string]parent.NetworkRef
	VMs               map[int]parent.VMInfo
	InstantiateErr    error
	LookupImageErr    error
	LookupVMErr       error
	TerminateErr      error
	CreateImageErr    error
	GetImageErr       error
	DeleteImageErr    error
	LastTerminateID   int
	LastTerminateHard bool
	LastDeleteImageID int
	NextVMID          int
	NextImageID       int
	LastInstantiate   parent.InstantiateRequest
	LastCreateImage   parent.CreateImageRequest
}

// New creates a fake OpenNebula client.
func New() *Client {
	return &Client{
		Templates:     map[string]parent.TemplateRef{},
		Images:        map[string]parent.ImageRef{},
		ImageInfoByID: map[int]parent.ImageInfo{},
		Datastores:    map[string]parent.DatastoreRef{},
		Networks:      map[string]parent.NetworkRef{},
		VMs:           map[int]parent.VMInfo{},
		NextVMID:      100,
		NextImageID:   200,
	}
}

func (c *Client) LookupDatastoreByName(_ context.Context, name string) (parent.DatastoreRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ref, ok := c.Datastores[name]
	if !ok {
		return parent.DatastoreRef{}, fmt.Errorf("%w: datastore %q", parent.ErrNotFound, name)
	}

	return ref, nil
}

func (c *Client) LookupTemplateByName(_ context.Context, name string) (parent.TemplateRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ref, ok := c.Templates[name]
	if !ok {
		return parent.TemplateRef{}, fmt.Errorf("%w: template %q", parent.ErrNotFound, name)
	}

	return ref, nil
}

func (c *Client) LookupImageByName(_ context.Context, name string) (parent.ImageRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.LookupImageErr != nil {
		return parent.ImageRef{}, c.LookupImageErr
	}

	ref, ok := c.Images[name]
	if !ok {
		return parent.ImageRef{}, fmt.Errorf("%w: image %q", parent.ErrNotFound, name)
	}

	return ref, nil
}

func (c *Client) LookupNetworksByName(_ context.Context, names []string) ([]parent.NetworkRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	results := make([]parent.NetworkRef, 0, len(names))
	for _, name := range names {
		ref, ok := c.Networks[name]
		if !ok {
			return nil, fmt.Errorf("%w: network %q", parent.ErrNotFound, name)
		}

		results = append(results, ref)
	}

	return results, nil
}

func (c *Client) CreateImage(_ context.Context, request parent.CreateImageRequest) (parent.ImageRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.CreateImageErr != nil {
		return parent.ImageRef{}, c.CreateImageErr
	}

	c.LastCreateImage = request

	imageID := c.NextImageID
	c.NextImageID++

	datastoreName := ""
	for _, ref := range c.Datastores {
		if ref.ID == request.DatastoreID {
			datastoreName = ref.Name
			break
		}
	}

	ref := parent.ImageRef{
		ID:        imageID,
		Name:      request.Name,
		Datastore: datastoreName,
	}
	info := parent.ImageInfo{
		ID:        imageID,
		Name:      request.Name,
		Datastore: datastoreName,
		State:     "READY",
		Source:    request.SourceURL,
	}
	if request.SourcePath != "" {
		info.Source = request.SourcePath
	}
	c.Images[request.Name] = ref
	c.ImageInfoByID[imageID] = info

	return ref, nil
}

func (c *Client) GetImage(_ context.Context, imageID int) (parent.ImageInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.GetImageErr != nil {
		return parent.ImageInfo{}, c.GetImageErr
	}

	info, ok := c.ImageInfoByID[imageID]
	if !ok {
		for _, ref := range c.Images {
			if ref.ID != imageID {
				continue
			}

			info = parent.ImageInfo{
				ID:        ref.ID,
				Name:      ref.Name,
				Datastore: ref.Datastore,
				SizeMiB:   ref.SizeMiB,
				State:     "READY",
			}
			c.ImageInfoByID[imageID] = info
			ok = true
			break
		}
	}

	if !ok {
		return parent.ImageInfo{}, fmt.Errorf("%w: image %d", parent.ErrNotFound, imageID)
	}

	return info, nil
}

func (c *Client) DeleteImage(_ context.Context, imageID int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.DeleteImageErr != nil {
		return c.DeleteImageErr
	}

	info, ok := c.ImageInfoByID[imageID]
	if !ok {
		return parent.ErrNotFound
	}

	c.LastDeleteImageID = imageID
	delete(c.ImageInfoByID, imageID)
	delete(c.Images, info.Name)

	return nil
}

func (c *Client) InstantiateTemplate(_ context.Context, request parent.InstantiateRequest) (parent.VMRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.InstantiateErr != nil {
		return parent.VMRef{}, c.InstantiateErr
	}

	c.LastInstantiate = request

	vmID := c.NextVMID
	c.NextVMID++
	c.VMs[vmID] = parent.VMInfo{
		ID:       vmID,
		Name:     request.VMName,
		State:    "ACTIVE",
		LCMState: "RUNNING",
	}

	return parent.VMRef{ID: vmID, Name: request.VMName}, nil
}

func (c *Client) GetVM(_ context.Context, vmID int) (parent.VMInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.LookupVMErr != nil {
		return parent.VMInfo{}, c.LookupVMErr
	}

	vm, ok := c.VMs[vmID]
	if !ok {
		return parent.VMInfo{}, fmt.Errorf("%w: vm %d", parent.ErrNotFound, vmID)
	}

	return vm, nil
}

func (c *Client) TerminateVM(_ context.Context, vmID int, hard bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.TerminateErr != nil {
		return c.TerminateErr
	}

	if _, ok := c.VMs[vmID]; !ok {
		return parent.ErrNotFound
	}

	c.LastTerminateID = vmID
	c.LastTerminateHard = hard
	delete(c.VMs, vmID)

	return nil
}
