/*
Copyright 2026 The Karpenter GCP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status

import (
	"context"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/karpenter-gcp/karpenter-provider-gcp/pkg/apis/v1alpha1"
	"github.com/karpenter-gcp/karpenter-provider-gcp/pkg/operator/options"
)

const invalidSelfHostedConfigReason = "InvalidSelfHostedConfig"

type Configuration struct{}

// Reconcile validates mode-dependent GCENodeClass configuration invariants that
// CRD CEL rules cannot enforce (for example, self-hosted NodeClasses without
// spec.startupScript that rely on image-baked bootstrap).
func (c *Configuration) Reconcile(ctx context.Context, nodeClass *v1alpha1.GCENodeClass) (reconcile.Result, error) {
	if !options.FromContext(ctx).IsSelfHosted() {
		nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeConfigurationReady)
		return reconcile.Result{}, nil
	}

	switch {
	case nodeClass.Spec.NetworkConfig == nil || nodeClass.Spec.NetworkConfig.Subnetwork == "":
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeConfigurationReady, invalidSelfHostedConfigReason,
			"spec.networkConfig.subnetwork is required in self-hosted mode")
	case nodeClass.Spec.KubeletConfiguration == nil || nodeClass.Spec.KubeletConfiguration.MaxPods == nil || *nodeClass.Spec.KubeletConfiguration.MaxPods < 1:
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeConfigurationReady, invalidSelfHostedConfigReason,
			"spec.kubeletConfiguration.maxPods must be set and >= 1 in self-hosted mode")
	case !slices.ContainsFunc(nodeClass.Spec.Disks, func(d v1alpha1.Disk) bool { return d.Boot }):
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeConfigurationReady, invalidSelfHostedConfigReason,
			"spec.disks must contain a boot disk in self-hosted mode")
	default:
		nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeConfigurationReady)
	}
	return reconcile.Result{}, nil
}
