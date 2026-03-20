// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"errors"
	"fmt"
	"time"

	"github.com/cosi-project/runtime/pkg/controller"
	"github.com/siderolabs/omni/client/pkg/infra/provision"
	"go.uber.org/zap"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

func (p *Provisioner) runProvisionStep(step string, fn func() error) error {
	start := time.Now()
	err := fn()

	if p.metrics != nil {
		outcome, class := classifyProvisionError(err)
		p.metrics.ObserveProvisionStep(step, outcome, time.Since(start))
		if outcome == "retry" {
			p.metrics.IncRetry(step, class)
		}
	}

	return err
}

func (p *Provisioner) observeDeprovision(err error, duration time.Duration) {
	if p.metrics == nil {
		return
	}

	outcome, class := classifyProvisionError(err)
	p.metrics.ObserveDeprovision(outcome, duration)
	if outcome == "retry" {
		p.metrics.IncRetry("deprovision", class)
	}
}

func (p *Provisioner) retryError(_ string, interval time.Duration, format string, args ...any) error {
	return provision.NewRetryErrorf(interval, format, args...)
}

func (p *Provisioner) clientError(step, action string, err error) error {
	class := opennebula.ClassifyError(err)
	if opennebula.IsRetryableClass(class) {
		return p.retryError(step, 10*time.Second, "%s: %w", action, err)
	}

	return fmt.Errorf("%s: %w", action, err)
}

func provisionLogger(logger *zap.Logger, machine *resources.Machine, requestID string, extraFields ...zap.Field) *zap.Logger {
	fields := []zap.Field{
		zap.String("request_id", requestID),
		zap.String("phase", machine.TypedSpec().Value.Phase),
		zap.String("vm_name", machine.TypedSpec().Value.VmName),
	}

	if vmID := int(machine.TypedSpec().Value.VmId); vmID != 0 {
		fields = append(fields, zap.Int("vm_id", vmID))
	}

	if templateName := machine.TypedSpec().Value.TemplateName; templateName != "" {
		fields = append(fields, zap.String("template_name", templateName))
	}

	if imageName := machine.TypedSpec().Value.ImageName; imageName != "" {
		fields = append(fields, zap.String("image_name", imageName))
	}

	if datastore := machine.TypedSpec().Value.Datastore; datastore != "" {
		fields = append(fields, zap.String("datastore", datastore))
	}
	if hypervisor := machine.TypedSpec().Value.ResolvedHypervisor; hypervisor != "" {
		fields = append(fields, zap.String("resolved_hypervisor", hypervisor))
	}
	if host := machine.TypedSpec().Value.ResolvedHostName; host != "" {
		fields = append(fields, zap.String("resolved_host", host))
	}
	if bootstrapProfile := machine.TypedSpec().Value.BootstrapProfile; bootstrapProfile != "" {
		fields = append(fields, zap.String("bootstrap_profile", bootstrapProfile))
	}

	if len(machine.TypedSpec().Value.NetworkNames) > 0 {
		fields = append(fields, zap.Strings("network_names", machine.TypedSpec().Value.NetworkNames))
	}

	fields = append(fields, extraFields...)
	return logger.With(fields...)
}

func classifyProvisionError(err error) (string, string) {
	if err == nil {
		return "success", string(opennebula.ErrorClassSuccess)
	}

	var requeueErr *controller.RequeueError
	if errors.As(err, &requeueErr) {
		if requeueErr.Err() == nil {
			return "retry", string(opennebula.ErrorClassRetryable)
		}

		return "retry", string(opennebula.ClassifyError(requeueErr.Err()))
	}

	return "error", string(opennebula.ClassifyError(err))
}
