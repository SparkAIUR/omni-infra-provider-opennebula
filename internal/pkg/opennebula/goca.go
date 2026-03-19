// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package opennebula

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/OpenNebula/one/src/oca/go/src/goca"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
)

// GOCAClient is the production OpenNebula adapter.
type GOCAClient struct {
	controller *goca.Controller
}

// NewClient creates a GOCA-backed OpenNebula adapter.
func NewClient(cfg config.Config, auth config.AuthConfig) (Client, error) {
	if auth.Session != "" {
		client, err := goca.NewClientFromConfig(goca.OneConfig{
			Token:    auth.Session,
			Endpoint: cfg.OpenNebula.Endpoint,
		})
		if err == nil {
			return &GOCAClient{controller: goca.NewController(client)}, nil
		}
	}

	client, err := goca.NewClientFromConfig(goca.NewConfig(auth.Username, auth.Password, cfg.OpenNebula.Endpoint))
	if err != nil {
		return nil, fmt.Errorf("create GOCA client: %w", err)
	}

	return &GOCAClient{controller: goca.NewController(client)}, nil
}

// LookupTemplateByName resolves a template by name.
func (c *GOCAClient) LookupTemplateByName(ctx context.Context, name string) (TemplateRef, error) {
	id, err := c.controller.Templates().ByNameContext(ctx, name)
	if err != nil {
		return TemplateRef{}, normalizeLookupError("template", name, err)
	}

	tpl, err := c.controller.Template(id).InfoContext(ctx, false, false)
	if err != nil {
		return TemplateRef{}, fmt.Errorf("info template %q: %w", name, err)
	}

	return TemplateRef{ID: id, Name: tpl.Name}, nil
}

// LookupImageByName resolves an image by name.
func (c *GOCAClient) LookupImageByName(ctx context.Context, name string) (ImageRef, error) {
	id, err := c.controller.Images().ByNameContext(ctx, name)
	if err != nil {
		return ImageRef{}, normalizeLookupError("image", name, err)
	}

	image, err := c.controller.Image(id).InfoContext(ctx, false)
	if err != nil {
		return ImageRef{}, fmt.Errorf("info image %q: %w", name, err)
	}

	datastore := ""
	if image.DatastoreID != nil {
		datastore = image.Datastore
	}

	return ImageRef{ID: id, Name: image.Name, Datastore: datastore, SizeMiB: image.Size / 1024}, nil
}

// LookupDatastoreByName resolves a datastore by name.
func (c *GOCAClient) LookupDatastoreByName(ctx context.Context, name string) (DatastoreRef, error) {
	id, err := c.controller.Datastores().ByNameContext(ctx, name)
	if err != nil {
		return DatastoreRef{}, normalizeLookupError("datastore", name, err)
	}

	datastore, err := c.controller.Datastore(id).InfoContext(ctx, false)
	if err != nil {
		return DatastoreRef{}, fmt.Errorf("info datastore %q: %w", name, err)
	}

	return DatastoreRef{ID: id, Name: datastore.Name, FreeMB: datastore.FreeMB}, nil
}

// LookupNetworksByName resolves a set of networks by name.
func (c *GOCAClient) LookupNetworksByName(ctx context.Context, names []string) ([]NetworkRef, error) {
	refs := make([]NetworkRef, 0, len(names))

	for _, name := range names {
		id, err := c.controller.VirtualNetworks().ByNameContext(ctx, name)
		if err != nil {
			return nil, normalizeLookupError("network", name, err)
		}

		network, err := c.controller.VirtualNetwork(id).InfoContext(ctx, false)
		if err != nil {
			return nil, fmt.Errorf("info network %q: %w", name, err)
		}

		refs = append(refs, NetworkRef{ID: id, Name: network.Name})
	}

	return refs, nil
}

// CreateImage imports an image into an OpenNebula datastore.
func (c *GOCAClient) CreateImage(ctx context.Context, request CreateImageRequest) (ImageRef, error) {
	if request.Driver == "" {
		request.Driver = "qcow2"
	}

	if request.Format == "" {
		request.Format = "qcow2"
	}

	source := request.SourceURL
	if strings.TrimSpace(request.SourcePath) != "" {
		source = request.SourcePath
	}

	templateBody := strings.Join([]string{
		fmt.Sprintf("NAME = %q", request.Name),
		`TYPE = "OS"`,
		fmt.Sprintf("PATH = %q", source),
		fmt.Sprintf("DRIVER = %q", request.Driver),
		fmt.Sprintf("FORMAT = %q", request.Format),
	}, "\n")

	imageID, err := c.controller.Images().CreateContext(ctx, templateBody, uint(request.DatastoreID))
	if err != nil {
		return ImageRef{}, fmt.Errorf("create image %q: %w", request.Name, err)
	}

	imageInfo, err := c.GetImage(ctx, imageID)
	if err != nil {
		return ImageRef{}, err
	}

	return ImageRef{
		ID:        imageInfo.ID,
		Name:      imageInfo.Name,
		Datastore: imageInfo.Datastore,
		SizeMiB:   imageInfo.SizeMiB,
	}, nil
}

// GetImage fetches the normalized image state.
func (c *GOCAClient) GetImage(ctx context.Context, imageID int) (ImageInfo, error) {
	imageInfo, err := c.controller.Image(imageID).InfoContext(ctx, false)
	if err != nil {
		return ImageInfo{}, normalizeLookupError("image", fmt.Sprint(imageID), err)
	}

	state, stateErr := imageInfo.StateString()
	if stateErr != nil {
		return ImageInfo{}, fmt.Errorf("state image %d: %w", imageID, stateErr)
	}

	datastore := ""
	if imageInfo.DatastoreID != nil {
		datastore = imageInfo.Datastore
	}

	return ImageInfo{
		ID:        imageInfo.ID,
		Name:      imageInfo.Name,
		Datastore: datastore,
		SizeMiB:   imageInfo.Size / 1024,
		State:     state,
		Source:    imageInfo.Source,
	}, nil
}

// DeleteImage deletes an image from OpenNebula.
func (c *GOCAClient) DeleteImage(ctx context.Context, imageID int) error {
	imageController := c.controller.Image(imageID)
	if _, err := imageController.InfoContext(ctx, false); err != nil {
		if errors.Is(normalizeLookupError("image", fmt.Sprint(imageID), err), ErrNotFound) {
			return nil
		}

		return fmt.Errorf("get image %d: %w", imageID, err)
	}

	if err := imageController.DeleteContext(ctx); err != nil {
		return fmt.Errorf("delete image %d: %w", imageID, err)
	}

	return nil
}

// InstantiateTemplate instantiates a VM from a template.
func (c *GOCAClient) InstantiateTemplate(ctx context.Context, request InstantiateRequest) (VMRef, error) {
	vmID, err := c.controller.Template(request.TemplateID).InstantiateContext(
		ctx,
		request.VMName,
		request.Pending,
		request.ExtraTemplate,
		request.CloneTemplate,
	)
	if err != nil {
		return VMRef{}, fmt.Errorf("instantiate template %d: %w", request.TemplateID, err)
	}

	return VMRef{ID: vmID, Name: request.VMName}, nil
}

// GetVM fetches the normalized VM state.
func (c *GOCAClient) GetVM(ctx context.Context, vmID int) (VMInfo, error) {
	vmInfo, err := c.controller.VM(vmID).InfoContext(ctx, false)
	if err != nil {
		return VMInfo{}, normalizeLookupError("vm", fmt.Sprint(vmID), err)
	}

	state, lcmState, stateErr := vmInfo.StateString()
	if stateErr != nil {
		return VMInfo{}, fmt.Errorf("state vm %d: %w", vmID, stateErr)
	}

	return VMInfo{
		ID:       vmInfo.ID,
		Name:     vmInfo.Name,
		State:    state,
		LCMState: lcmState,
	}, nil
}

// TerminateVM terminates an OpenNebula VM.
func (c *GOCAClient) TerminateVM(ctx context.Context, vmID int, hard bool) error {
	vmController := c.controller.VM(vmID)
	if _, err := vmController.InfoContext(ctx, false); err != nil {
		if errors.Is(normalizeLookupError("vm", fmt.Sprint(vmID), err), ErrNotFound) {
			return nil
		}

		return fmt.Errorf("get vm %d: %w", vmID, err)
	}

	if hard {
		if err := vmController.TerminateHardContext(ctx); err != nil {
			return fmt.Errorf("terminate hard vm %d: %w", vmID, err)
		}

		return nil
	}

	if err := vmController.TerminateContext(ctx); err != nil {
		return fmt.Errorf("terminate vm %d: %w", vmID, err)
	}

	return nil
}

func normalizeLookupError(kind, name string, err error) error {
	if err == nil {
		return nil
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "resource not found") || strings.Contains(msg, "not found") {
		return fmt.Errorf("%w: %s %q", ErrNotFound, kind, name)
	}

	return fmt.Errorf("lookup %s %q: %w", kind, name, err)
}
