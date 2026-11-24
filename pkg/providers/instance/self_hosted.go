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

package instance

import (
	"context"
	"fmt"

	"google.golang.org/api/compute/v1"

	"github.com/karpenter-gcp/karpenter-provider-gcp/pkg/apis/v1alpha1"
	"github.com/karpenter-gcp/karpenter-provider-gcp/pkg/providers/subnet"
)

// This file contains the self-hosted launch path. Node bootstrap is owned by the
// NodeClass startup script or baked into the image, so the instance carries no GKE
// bootstrap metadata, tags, or labels.

// setupSelfHostedNetworkInterfaces builds NICs from spec.networkConfig: the primary
// subnetwork is required and each interface's network is derived via the Compute API.
// Nodes are private by default; alias IP ranges use podRange (resolved from
// spec.subnetRangeNames or deprecated spec.subnetRangeName) since there is no cluster
// IP allocation policy to fall back to.
func (p *DefaultProvider) setupSelfHostedNetworkInterfaces(ctx context.Context, nodeClass *v1alpha1.GCENodeClass, podRange string) ([]*compute.NetworkInterface, error) {
	cfg := nodeClass.Spec.NetworkConfig
	if cfg == nil || cfg.Subnetwork == "" {
		return nil, fmt.Errorf("GCENodeClass %s: spec.networkConfig.subnetwork is required in self-hosted mode", nodeClass.Name)
	}
	project, region, name, err := subnet.ParseSubnetworkRef(cfg.Subnetwork, p.projectID, p.region)
	if err != nil {
		return nil, err
	}
	network, err := p.networkForSubnetwork(ctx, project, region, name)
	if err != nil {
		return nil, err
	}

	disableExternal := cfg.EnablePrivateNodes == nil || *cfg.EnablePrivateNodes
	nic := networkInterfaceType(nodeClass)

	primary := &compute.NetworkInterface{
		Network:    network,
		Subnetwork: canonicalSubnetworkRef(project, region, name),
		NicType:    nic,
	}
	if podRange != "" {
		primary.AliasIpRanges = []*compute.AliasIpRange{{
			IpCidrRange:         fmt.Sprintf("/%d", podCIDRRange(nodeClass.GetMaxPods())),
			SubnetworkRangeName: podRange,
		}}
	}
	applyAccessConfig(primary, disableExternal)

	ifaces := []*compute.NetworkInterface{primary}
	for _, override := range cfg.AdditionalNetworkInterfaces {
		project, region, name, err := subnet.ParseSubnetworkRef(override.Subnetwork, p.projectID, p.region)
		if err != nil {
			return nil, err
		}
		ifaceNetwork := override.Network
		if ifaceNetwork == "" {
			if ifaceNetwork, err = p.networkForSubnetwork(ctx, project, region, name); err != nil {
				return nil, err
			}
		}
		iface := &compute.NetworkInterface{
			Network: ifaceNetwork,
			// The raw spec value may be a bare name, which Instances.Insert rejects.
			Subnetwork: canonicalSubnetworkRef(project, region, name),
			NicType:    nic,
		}
		applyAccessConfig(iface, disableExternal)
		ifaces = append(ifaces, iface)
	}
	return ifaces, nil
}

// networkForSubnetwork returns the subnetwork's network, caching the effectively
// immutable mapping so launch retries do not re-fetch it.
func (p *DefaultProvider) networkForSubnetwork(ctx context.Context, project, region, name string) (string, error) {
	ref := canonicalSubnetworkRef(project, region, name)
	if network, ok := p.subnetworkNetworks.Load(ref); ok {
		return network.(string), nil
	}
	subnet, err := p.computeService.Subnetworks.Get(project, region, name).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("fetching subnetwork %q to derive its network: %w", ref, err)
	}
	p.subnetworkNetworks.Store(ref, subnet.Network)
	return subnet.Network, nil
}

// canonicalSubnetworkRef builds the partial-URL subnetwork reference accepted by
// Instances.Insert.
func canonicalSubnetworkRef(project, region, name string) string {
	return fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", project, region, name)
}
