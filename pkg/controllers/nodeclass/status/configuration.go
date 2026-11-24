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

	if msg := validateSelfHostedNodeClass(nodeClass); msg != "" {
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeConfigurationReady, invalidSelfHostedConfigReason, msg)
		return reconcile.Result{}, nil
	}
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeConfigurationReady)
	return reconcile.Result{}, nil
}

func validateSelfHostedNodeClass(nodeClass *v1alpha1.GCENodeClass) string {
	switch {
	case nodeClass.Spec.NetworkConfig == nil || nodeClass.Spec.NetworkConfig.Subnetwork == "":
		return "spec.networkConfig.subnetwork is required in self-hosted mode"
	case !hasValidMaxPods(nodeClass.Spec.KubeletConfiguration):
		return "spec.kubeletConfiguration.maxPods must be set and >= 1 in self-hosted mode"
	case !hasBootDisk(nodeClass.Spec.Disks):
		return "spec.disks must contain a boot disk in self-hosted mode"
	case nodeClass.Spec.AutoGPUTaint:
		return "spec.autoGPUTaint is not supported in self-hosted mode"
	case hasSecondaryBootDisk(nodeClass.Spec.Disks):
		return "spec.disks[].secondaryBootImage and secondaryBootMode are not supported in self-hosted mode"
	case hasUnsupportedSelfHostedKubeletConfig(nodeClass.Spec.KubeletConfiguration):
		return "spec.kubeletConfiguration only supports binpacking fields (maxPods, podsPerCore, systemReserved, kubeReserved, evictionHard) in self-hosted mode"
	default:
		return ""
	}
}

func hasValidMaxPods(kc *v1alpha1.KubeletConfiguration) bool {
	return kc != nil && kc.MaxPods != nil && *kc.MaxPods >= 1
}

func hasBootDisk(disks []v1alpha1.Disk) bool {
	return slices.ContainsFunc(disks, func(d v1alpha1.Disk) bool { return d.Boot })
}

func hasSecondaryBootDisk(disks []v1alpha1.Disk) bool {
	return slices.ContainsFunc(disks, func(d v1alpha1.Disk) bool {
		return d.SecondaryBootImage != "" || d.SecondaryBootMode != ""
	})
}

// hasUnsupportedSelfHostedKubeletConfig mirrors the startupScript CEL rule for
// image-baked self-hosted GCENodeClasses: any kubeletConfiguration field outside
// the binpacking allowlist is delivered only via GKE kubelet-config metadata.
func hasUnsupportedSelfHostedKubeletConfig(kc *v1alpha1.KubeletConfiguration) bool {
	if kc == nil {
		return false
	}
	return slices.Contains([]bool{
		len(kc.ClusterDNS) > 0,
		len(kc.EvictionSoft) > 0,
		len(kc.EvictionSoftGracePeriod) > 0,
		kc.EvictionMaxPodGracePeriod != nil,
		kc.ImageGCHighThresholdPercent != nil,
		kc.ImageGCLowThresholdPercent != nil,
		kc.CPUCFSQuota != nil,
		kc.CPUManagerPolicy != "",
		kc.SerializeImagePulls != nil,
		kc.MaxParallelImagePulls != nil,
		kc.FailSwapOn != nil,
		kc.MemorySwap != nil,
		kc.ShutdownGracePeriod != nil,
		kc.ShutdownGracePeriodCriticalPods != nil,
	}, true)
}
