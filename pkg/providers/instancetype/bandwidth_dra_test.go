/*
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

// White-box test: package `instancetype` (not `_test`) so the unexported
// bandwidthSlicesFor is reachable. Coexists with the ginkgo suite_test.go.
package instancetype

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	. "github.com/onsi/gomega"
)

// networkInfo builds a NetworkInfo with a single network card at index 0 carrying the given
// baseline/peak Gbps. Mirrors the fake-EC2 pointer idiom (aws.Float64 / aws.Int32).
func networkInfo(defaultCardIdx *int32, baseline, peak *float64) *ec2types.NetworkInfo {
	return &ec2types.NetworkInfo{
		DefaultNetworkCardIndex: defaultCardIdx,
		NetworkCards: []ec2types.NetworkCardInfo{{
			NetworkCardIndex:        aws.Int32(0),
			BaselineBandwidthInGbps: baseline,
			PeakBandwidthInGbps:     peak,
		}},
	}
}

func TestBandwidthSlicesFor(t *testing.T) {
	cases := []struct {
		name        string
		info        ec2types.InstanceTypeInfo
		wantDevices int    // 0 means expect nil slice
		wantPool    string // asserted only when wantDevices > 0
	}{
		{
			name: "guaranteed size (baseline==peak==15) yields 15 exclusive 1-Gbps devices",
			info: ec2types.InstanceTypeInfo{
				InstanceType: "c8i.8xlarge",
				NetworkInfo:  networkInfo(aws.Int32(0), aws.Float64(15.0), aws.Float64(15.0)),
			},
			wantDevices: 15,
			wantPool:    "c8i.8xlarge-bandwidth",
		},
		{
			name: "guaranteed 22.5 floors to 22 devices",
			info: ec2types.InstanceTypeInfo{
				InstanceType: "c8i.12xlarge",
				NetworkInfo:  networkInfo(aws.Int32(0), aws.Float64(22.5), aws.Float64(22.5)),
			},
			wantDevices: 22,
			wantPool:    "c8i.12xlarge-bandwidth",
		},
		{
			name: "burstable size (baseline 7.5 < peak 15) yields nil",
			info: ec2types.InstanceTypeInfo{
				InstanceType: "c8i.4xlarge",
				NetworkInfo:  networkInfo(aws.Int32(0), aws.Float64(7.5), aws.Float64(15.0)),
			},
			wantDevices: 0,
		},
		{
			name:        "nil NetworkInfo yields nil",
			info:        ec2types.InstanceTypeInfo{InstanceType: "c8i.8xlarge", NetworkInfo: nil},
			wantDevices: 0,
		},
		{
			name: "nil DefaultNetworkCardIndex yields nil",
			info: ec2types.InstanceTypeInfo{
				InstanceType: "c8i.8xlarge",
				NetworkInfo:  networkInfo(nil, aws.Float64(15.0), aws.Float64(15.0)),
			},
			wantDevices: 0,
		},
		{
			name: "baseline floors to 0 yields nil",
			info: ec2types.InstanceTypeInfo{
				InstanceType: "c8i.tiny",
				NetworkInfo:  networkInfo(aws.Int32(0), aws.Float64(0.4), aws.Float64(0.4)),
			},
			wantDevices: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := bandwidthSlicesFor(tc.info)

			if tc.wantDevices == 0 {
				g.Expect(got).To(BeNil())
				return
			}

			g.Expect(got).To(HaveLen(1))
			slice := got[0]
			g.Expect(slice.Driver.Value()).To(Equal(bandwidthDRADriver)) // "bandwidth.networking.aws"
			g.Expect(slice.Pool.Name.Value()).To(Equal(tc.wantPool))
			g.Expect(slice.SharedCounters).To(BeEmpty())
			g.Expect(slice.Devices).To(HaveLen(tc.wantDevices))

			// Every device is exclusive (count-only model) and uniquely named within the pool.
			seen := map[string]struct{}{}
			for _, d := range slice.Devices {
				g.Expect(d.AllowMultipleAllocations).To(BeFalse())
				name := d.Name.Value()
				_, dup := seen[name]
				g.Expect(dup).To(BeFalse(), "device names must be unique within the pool")
				seen[name] = struct{}{}
			}
		})
	}
}
