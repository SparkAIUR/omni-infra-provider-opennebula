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
	mu                  sync.Mutex
	Templates           map[string]parent.TemplateRef
	Images              map[string]parent.ImageRef
	ImageInfoByID       map[int]parent.ImageInfo
	Datastores          map[string]parent.DatastoreRef
	Networks            map[string]parent.NetworkRef
	Hypervisors         []string
	Hosts               []parent.HostInfo
	VMs                 map[int]parent.VMInfo
	InstantiateErr      error
	LookupImageErr      error
	LookupVMErr         error
	TerminateErr        error
	ForceDeleteErr      error
	CreateImageErr      error
	GetImageErr         error
	DeleteImageErr      error
	LastTerminateID     int
	LastTerminateHard   bool
	LastForceDeleteID   int
	LastDeleteImageID   int
	NextVMID            int
	NextImageID         int
	LastInstantiate     parent.InstantiateRequest
	LastCreateImage     parent.CreateImageRequest
	TerminateLeavesVM   bool
	ForceDeleteLeavesVM bool
}

// New creates a fake OpenNebula client.
func New() *Client {
	return &Client{
		Templates:     map[string]parent.TemplateRef{},
		Images:        map[string]parent.ImageRef{},
		ImageInfoByID: map[int]parent.ImageInfo{},
		Datastores:    map[string]parent.DatastoreRef{},
		Networks:      map[string]parent.NetworkRef{},
		Hypervisors:   []string{"qemu"},
		Hosts:         nil,
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

func (c *Client) ListHosts(_ context.Context, request parent.HostListRequest) ([]parent.HostInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.Hosts) > 0 {
		hosts := make([]parent.HostInfo, 0, len(c.Hosts))
		for _, host := range c.Hosts {
			if request.ResourcePool != "" && host.ClusterName != request.ResourcePool {
				continue
			}
			hosts = append(hosts, host)
		}

		return hosts, nil
	}

	hosts := make([]parent.HostInfo, 0, len(c.Hypervisors))
	for idx, hypervisor := range c.Hypervisors {
		hosts = append(hosts, parent.HostInfo{
			ID:             idx + 1,
			Name:           fmt.Sprintf("host-%d", idx+1),
			ClusterID:      1,
			ClusterName:    "default",
			Hypervisor:     hypervisor,
			Enabled:        true,
			Schedulable:    true,
			CPUTotal:       1600,
			CPUUsed:        idx * 100,
			MemoryTotalMiB: 32768,
			MemoryUsedMiB:  idx * 1024,
			RunningVMs:     idx,
		})
	}

	return hosts, nil
}

func (c *Client) ResolveHypervisor(_ context.Context, _ parent.HypervisorResolveRequest) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.Hosts) > 0 {
		foundQEMU := false
		for _, host := range c.Hosts {
			if !host.Enabled || !host.Schedulable {
				continue
			}

			switch host.Hypervisor {
			case "kvm":
				return "kvm", nil
			case "qemu":
				foundQEMU = true
			}
		}

		if foundQEMU {
			return "qemu", nil
		}

		return "", fmt.Errorf("%w: neither kvm nor qemu was found on eligible hosts", parent.ErrPolicy)
	}

	foundQEMU := false

	for _, hypervisor := range c.Hypervisors {
		switch hypervisor {
		case "kvm":
			return "kvm", nil
		case "qemu":
			foundQEMU = true
		}
	}

	if foundQEMU {
		return "qemu", nil
	}

	return "", fmt.Errorf("%w: neither kvm nor qemu was found on eligible hosts", parent.ErrPolicy)
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
	if c.TerminateLeavesVM {
		vm := c.VMs[vmID]
		vm.State = "ACTIVE"
		vm.LCMState = "SHUTDOWN"
		c.VMs[vmID] = vm
		return nil
	}

	delete(c.VMs, vmID)

	return nil
}

func (c *Client) ForceDeleteVM(_ context.Context, vmID int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ForceDeleteErr != nil {
		return c.ForceDeleteErr
	}

	if _, ok := c.VMs[vmID]; !ok {
		return parent.ErrNotFound
	}

	c.LastForceDeleteID = vmID
	if c.ForceDeleteLeavesVM {
		return nil
	}

	delete(c.VMs, vmID)

	return nil
}
