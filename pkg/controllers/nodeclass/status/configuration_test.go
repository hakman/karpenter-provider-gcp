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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/karpenter-gcp/karpenter-provider-gcp/pkg/apis/v1alpha1"
	"github.com/karpenter-gcp/karpenter-provider-gcp/pkg/operator/options"
)

func TestConfigurationReconcile(t *testing.T) {
	t.Parallel()

	selfHostedCtx := options.ToContext(context.Background(), &options.Options{
		ProvisionMode: options.ProvisionModeSelfHosted,
	})
	validSelfHosted := func() *v1alpha1.GCENodeClass {
		return &v1alpha1.GCENodeClass{
			Spec: v1alpha1.GCENodeClassSpec{
				NetworkConfig:        &v1alpha1.NetworkConfig{Subnetwork: "nodes"},
				KubeletConfiguration: &v1alpha1.KubeletConfiguration{MaxPods: ptr.To(int32(110))},
				Disks:                []v1alpha1.Disk{{Boot: true, SizeGiB: 100}},
			},
		}
	}

	t.Run("gke mode marks configuration ready without self-hosted fields", func(t *testing.T) {
		t.Parallel()
		nc := &v1alpha1.GCENodeClass{}
		_, err := (&Configuration{}).Reconcile(context.Background(), nc)
		require.NoError(t, err)
		require.True(t, nc.StatusConditions().IsTrue(v1alpha1.ConditionTypeConfigurationReady))
	})

	t.Run("self-hosted valid image-baked nodeClass without startupScript is ready", func(t *testing.T) {
		t.Parallel()
		nc := validSelfHosted()
		_, err := (&Configuration{}).Reconcile(selfHostedCtx, nc)
		require.NoError(t, err)
		require.True(t, nc.StatusConditions().IsTrue(v1alpha1.ConditionTypeConfigurationReady))
	})

	cases := []struct {
		name        string
		mutate      func(*v1alpha1.GCENodeClass)
		wantMessage string
	}{
		{
			name:        "missing networkConfig",
			mutate:      func(nc *v1alpha1.GCENodeClass) { nc.Spec.NetworkConfig = nil },
			wantMessage: "spec.networkConfig.subnetwork is required in self-hosted mode",
		},
		{
			name:        "missing maxPods",
			mutate:      func(nc *v1alpha1.GCENodeClass) { nc.Spec.KubeletConfiguration = nil },
			wantMessage: "spec.kubeletConfiguration.maxPods must be set and >= 1 in self-hosted mode",
		},
		{
			name:        "zero maxPods",
			mutate:      func(nc *v1alpha1.GCENodeClass) { nc.Spec.KubeletConfiguration.MaxPods = ptr.To(int32(0)) },
			wantMessage: "spec.kubeletConfiguration.maxPods must be set and >= 1 in self-hosted mode",
		},
		{
			name:        "missing boot disk",
			mutate:      func(nc *v1alpha1.GCENodeClass) { nc.Spec.Disks = []v1alpha1.Disk{{SizeGiB: 100}} },
			wantMessage: "spec.disks must contain a boot disk in self-hosted mode",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nc := validSelfHosted()
			tc.mutate(nc)

			_, err := (&Configuration{}).Reconcile(selfHostedCtx, nc)
			require.NoError(t, err)
			cond := nc.StatusConditions().Get(v1alpha1.ConditionTypeConfigurationReady)
			require.True(t, cond.IsFalse())
			require.Equal(t, invalidSelfHostedConfigReason, cond.Reason)
			require.Equal(t, tc.wantMessage, cond.Message)
		})
	}
}
