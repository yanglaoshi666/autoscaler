/*
Copyright 2023 The Kubernetes Authors.

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

package actuation

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	testprovider "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/test"
	"k8s.io/autoscaler/cluster-autoscaler/config"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown/budgets"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown/deletiontracker"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown/status"
	. "k8s.io/autoscaler/cluster-autoscaler/core/test"
	kube_util "k8s.io/autoscaler/cluster-autoscaler/utils/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	schedulerframework "k8s.io/kubernetes/pkg/scheduler/framework"
)

func TestScheduleDeletion(t *testing.T) {
	testNg := testprovider.NewTestNodeGroup("test", 0, 100, 3, true, false, "n1-standard-2", nil, nil)
	atomic2 := sizedNodeGroup("atomic-2", 2, true)
	atomic4 := sizedNodeGroup("atomic-4", 4, true)

	testCases := []struct {
		name                  string
		toSchedule            []*budgets.NodeGroupView
		toAbort               []*budgets.NodeGroupView
		toScheduleAfterAbort  []*budgets.NodeGroupView
		wantDeleted           int
		wantNodeDeleteResults map[string]status.NodeDeleteResult
	}{
		{
			name:       "no nodes",
			toSchedule: []*budgets.NodeGroupView{},
		},
		{
			name:                 "individual nodes are deleted right away",
			toSchedule:           generateNodeGroupViewList(testNg, 0, 3),
			toAbort:              generateNodeGroupViewList(testNg, 3, 6),
			toScheduleAfterAbort: generateNodeGroupViewList(testNg, 6, 9),
			wantDeleted:          6,
			wantNodeDeleteResults: map[string]status.NodeDeleteResult{
				"test-node-3": {ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError},
				"test-node-4": {ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError},
				"test-node-5": {ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError},
			},
		},
		{
			name: "whole atomic node groups deleted",
			toSchedule: mergeLists(
				generateNodeGroupViewList(atomic4, 0, 1),
				generateNodeGroupViewList(atomic2, 0, 1),
				generateNodeGroupViewList(atomic4, 1, 2),
				generateNodeGroupViewList(atomic2, 1, 2),
				generateNodeGroupViewList(atomic4, 2, 4),
			),
			wantDeleted: 6,
		},
		{
			name: "atomic node group aborted in the process",
			toSchedule: mergeLists(
				generateNodeGroupViewList(atomic4, 0, 1),
				generateNodeGroupViewList(atomic2, 0, 1),
				generateNodeGroupViewList(atomic4, 1, 2),
				generateNodeGroupViewList(atomic2, 1, 2),
			),
			toAbort:              generateNodeGroupViewList(atomic4, 2, 3),
			toScheduleAfterAbort: generateNodeGroupViewList(atomic4, 3, 4),
			wantDeleted:          2,
			wantNodeDeleteResults: map[string]status.NodeDeleteResult{
				"atomic-4-node-0": {ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError},
				"atomic-4-node-1": {ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError},
				"atomic-4-node-2": {ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError},
				"atomic-4-node-3": {ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError},
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			provider := testprovider.NewTestCloudProvider(nil, func(nodeGroup string, node string) error {
				return nil
			})
			for _, bucket := range append(append(tc.toSchedule, tc.toAbort...), tc.toScheduleAfterAbort...) {
				bucket.Group.(*testprovider.TestNodeGroup).SetCloudProvider(provider)
				provider.InsertNodeGroup(bucket.Group)
				for _, node := range bucket.Nodes {
					provider.AddNode(bucket.Group.Id(), node)
				}
			}

			batcher := &countingBatcher{}
			tracker := deletiontracker.NewNodeDeletionTracker(0)
			opts := config.AutoscalingOptions{}
			fakeClient := &fake.Clientset{}
			podLister := kube_util.NewTestPodLister([]*apiv1.Pod{})
			pdbLister := kube_util.NewTestPodDisruptionBudgetLister([]*policyv1.PodDisruptionBudget{})
			dsLister, err := kube_util.NewTestDaemonSetLister([]*appsv1.DaemonSet{})
			if err != nil {
				t.Fatalf("Couldn't create daemonset lister")
			}
			registry := kube_util.NewListerRegistry(nil, nil, podLister, pdbLister, dsLister, nil, nil, nil, nil)
			ctx, err := NewScaleTestAutoscalingContext(opts, fakeClient, registry, provider, nil, nil)
			if err != nil {
				t.Fatalf("Couldn't set up autoscaling context: %v", err)
			}
			scheduler := NewGroupDeletionScheduler(&ctx, tracker, batcher, Evictor{EvictionRetryTime: 0, DsEvictionRetryTime: 0, DsEvictionEmptyNodeTimeout: 0, PodEvictionHeadroom: DefaultPodEvictionHeadroom})

			if err := scheduleAll(tc.toSchedule, scheduler); err != nil {
				t.Fatal(err)
			}
			for _, bucket := range tc.toAbort {
				for _, node := range bucket.Nodes {
					nodeDeleteResult := status.NodeDeleteResult{ResultType: status.NodeDeleteErrorFailedToDelete, Err: cmpopts.AnyError}
					scheduler.AbortNodeDeletion(node, bucket.Group.Id(), false, "simulated abort", nodeDeleteResult)
				}
			}
			if err := scheduleAll(tc.toScheduleAfterAbort, scheduler); err != nil {
				t.Fatal(err)
			}

			if batcher.addedNodes != tc.wantDeleted {
				t.Errorf("Incorrect number of deleted nodes, want %v but got %v", tc.wantDeleted, batcher.addedNodes)
			}
			gotDeletionResult, _ := tracker.DeletionResults()
			if diff := cmp.Diff(tc.wantNodeDeleteResults, gotDeletionResult, cmpopts.EquateEmpty(), cmpopts.EquateErrors()); diff != "" {
				t.Errorf("NodeDeleteResults diff (-want +got):\n%s", diff)
			}
		})
	}
}

type countingBatcher struct {
	addedNodes int
}

func (b *countingBatcher) AddNodes(nodes []*apiv1.Node, nodeGroup cloudprovider.NodeGroup, drain bool) {
	b.addedNodes += len(nodes)
}

func scheduleAll(toSchedule []*budgets.NodeGroupView, scheduler *GroupDeletionScheduler) error {
	for _, bucket := range toSchedule {
		bucketSize, err := bucket.Group.TargetSize()
		if err != nil {
			return fmt.Errorf("failed to get target size for node group %q: %s", bucket.Group.Id(), err)
		}
		for _, node := range bucket.Nodes {
			scheduler.ScheduleDeletion(infoForNode(node), bucket.Group, bucketSize, false)
		}
	}
	return nil
}

func infoForNode(n *apiv1.Node) *framework.NodeInfo {
	info := schedulerframework.NewNodeInfo()
	info.SetNode(n)
	return info
}

func mergeLists(lists ...[]*budgets.NodeGroupView) []*budgets.NodeGroupView {
	merged := []*budgets.NodeGroupView{}
	for _, l := range lists {
		merged = append(merged, l...)
	}
	return merged
}
