/*
Copyright 2016 The Kubernetes Authors.

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

package kubernetes

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	client "k8s.io/client-go/kubernetes"
	v1appslister "k8s.io/client-go/listers/apps/v1"
	v1batchlister "k8s.io/client-go/listers/batch/v1"
	v1lister "k8s.io/client-go/listers/core/v1"
	v1policylister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"
	podv1 "k8s.io/kubernetes/pkg/api/v1/pod"
)

// ListerRegistry is a registry providing various listers to list pods or nodes matching conditions
type ListerRegistry interface {
	AllNodeLister() NodeLister
	ReadyNodeLister() NodeLister
	AllPodLister() PodLister
	PodDisruptionBudgetLister() PodDisruptionBudgetLister
	DaemonSetLister() v1appslister.DaemonSetLister
	ReplicationControllerLister() v1lister.ReplicationControllerLister
	JobLister() v1batchlister.JobLister
	ReplicaSetLister() v1appslister.ReplicaSetLister
	StatefulSetLister() v1appslister.StatefulSetLister
}

type listerRegistryImpl struct {
	allNodeLister               NodeLister
	readyNodeLister             NodeLister
	allPodLister                PodLister
	podDisruptionBudgetLister   PodDisruptionBudgetLister
	daemonSetLister             v1appslister.DaemonSetLister
	replicationControllerLister v1lister.ReplicationControllerLister
	jobLister                   v1batchlister.JobLister
	replicaSetLister            v1appslister.ReplicaSetLister
	statefulSetLister           v1appslister.StatefulSetLister
}

// NewListerRegistry returns a registry providing various listers to list pods or nodes matching conditions
func NewListerRegistry(allNode NodeLister, readyNode NodeLister, allPodLister PodLister, podDisruptionBudgetLister PodDisruptionBudgetLister,
	daemonSetLister v1appslister.DaemonSetLister, replicationControllerLister v1lister.ReplicationControllerLister,
	jobLister v1batchlister.JobLister, replicaSetLister v1appslister.ReplicaSetLister,
	statefulSetLister v1appslister.StatefulSetLister) ListerRegistry {
	return listerRegistryImpl{
		allNodeLister:               allNode,
		readyNodeLister:             readyNode,
		allPodLister:                allPodLister,
		podDisruptionBudgetLister:   podDisruptionBudgetLister,
		daemonSetLister:             daemonSetLister,
		replicationControllerLister: replicationControllerLister,
		jobLister:                   jobLister,
		replicaSetLister:            replicaSetLister,
		statefulSetLister:           statefulSetLister,
	}
}

// NewListerRegistryWithDefaultListers returns a registry filled with listers of the default implementations
func NewListerRegistryWithDefaultListers(kubeClient client.Interface, stopChannel <-chan struct{}) ListerRegistry {
	allPodLister := NewAllPodLister(kubeClient, stopChannel)
	readyNodeLister := NewReadyNodeLister(kubeClient, stopChannel)
	allNodeLister := NewAllNodeLister(kubeClient, stopChannel)
	podDisruptionBudgetLister := NewPodDisruptionBudgetLister(kubeClient, stopChannel)
	daemonSetLister := NewDaemonSetLister(kubeClient, stopChannel)
	replicationControllerLister := NewReplicationControllerLister(kubeClient, stopChannel)
	jobLister := NewJobLister(kubeClient, stopChannel)
	replicaSetLister := NewReplicaSetLister(kubeClient, stopChannel)
	statefulSetLister := NewStatefulSetLister(kubeClient, stopChannel)
	return NewListerRegistry(allNodeLister, readyNodeLister, allPodLister,
		podDisruptionBudgetLister, daemonSetLister, replicationControllerLister,
		jobLister, replicaSetLister, statefulSetLister)
}

// AllPodLister returns the AllPodLister registered to this registry
func (r listerRegistryImpl) AllPodLister() PodLister {
	return r.allPodLister
}

// AllNodeLister returns the AllNodeLister registered to this registry
func (r listerRegistryImpl) AllNodeLister() NodeLister {
	return r.allNodeLister
}

// ReadyNodeLister returns the ReadyNodeLister registered to this registry
func (r listerRegistryImpl) ReadyNodeLister() NodeLister {
	return r.readyNodeLister
}

// PodDisruptionBudgetLister returns the podDisruptionBudgetLister registered to this registry
func (r listerRegistryImpl) PodDisruptionBudgetLister() PodDisruptionBudgetLister {
	return r.podDisruptionBudgetLister
}

// DaemonSetLister returns the daemonSetLister registered to this registry
func (r listerRegistryImpl) DaemonSetLister() v1appslister.DaemonSetLister {
	return r.daemonSetLister
}

// ReplicationControllerLister returns the replicationControllerLister registered to this registry
func (r listerRegistryImpl) ReplicationControllerLister() v1lister.ReplicationControllerLister {
	return r.replicationControllerLister
}

// JobLister returns the jobLister registered to this registry
func (r listerRegistryImpl) JobLister() v1batchlister.JobLister {
	return r.jobLister
}

// ReplicaSetLister returns the replicaSetLister registered to this registry
func (r listerRegistryImpl) ReplicaSetLister() v1appslister.ReplicaSetLister {
	return r.replicaSetLister
}

// StatefulSetLister returns the statefulSetLister registered to this registry
func (r listerRegistryImpl) StatefulSetLister() v1appslister.StatefulSetLister {
	return r.statefulSetLister
}

// PodLister lists all pods.
// To filter out the scheduled or unschedulable pods the helper methods ScheduledPods and UnschedulablePods should be used.
type PodLister interface {
	List() ([]*apiv1.Pod, error)
}

// ScheduledPods is a helper method that returns all scheduled pods from given pod list.
func ScheduledPods(allPods []*apiv1.Pod) []*apiv1.Pod {
	var scheduledPods []*apiv1.Pod
	for _, pod := range allPods {
		if pod.Spec.NodeName != "" {
			scheduledPods = append(scheduledPods, pod)
			continue
		}
	}
	return scheduledPods
}

// UnschedulablePods is a helper method that returns all unschedulable pods from given pod list.
func UnschedulablePods(allPods []*apiv1.Pod) []*apiv1.Pod {
	var unschedulablePods []*apiv1.Pod
	for _, pod := range allPods {
		if pod.Spec.NodeName == "" {
			_, condition := podv1.GetPodCondition(&pod.Status, apiv1.PodScheduled)
			if condition != nil && condition.Status == apiv1.ConditionFalse && condition.Reason == apiv1.PodReasonUnschedulable {
				if pod.GetDeletionTimestamp() == nil {
					unschedulablePods = append(unschedulablePods, pod)
				}
			}
		}
	}
	return unschedulablePods
}

// AllPodLister lists all pods.
type AllPodLister struct {
	podLister v1lister.PodLister
}

// List returns all scheduled pods.
func (lister *AllPodLister) List() ([]*apiv1.Pod, error) {
	return lister.podLister.List(labels.Everything())
}

// NewAllPodLister builds AllPodLister
func NewAllPodLister(kubeClient client.Interface, stopchannel <-chan struct{}) PodLister {
	selector := fields.ParseSelectorOrDie("status.phase!=" +
		string(apiv1.PodSucceeded) + ",status.phase!=" + string(apiv1.PodFailed))
	podListWatch := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "pods", apiv1.NamespaceAll, selector)
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(podListWatch, &apiv1.Pod{}, time.Hour)
	podLister := v1lister.NewPodLister(store)
	go reflector.Run(stopchannel)

	return &AllPodLister{
		podLister: podLister,
	}
}

// NodeLister lists nodes.
type NodeLister interface {
	List() ([]*apiv1.Node, error)
	Get(name string) (*apiv1.Node, error)
}

// nodeLister implementation.
type nodeListerImpl struct {
	nodeLister v1lister.NodeLister
	filter     func(*apiv1.Node) bool
}

// NewReadyNodeLister builds a node lister that returns only ready nodes.
func NewReadyNodeLister(kubeClient client.Interface, stopChannel <-chan struct{}) NodeLister {
	return NewNodeLister(kubeClient, IsNodeReadyAndSchedulable, stopChannel)
}

// NewAllNodeLister builds a node lister that returns all nodes (ready and unready).
func NewAllNodeLister(kubeClient client.Interface, stopChannel <-chan struct{}) NodeLister {
	return NewNodeLister(kubeClient, nil, stopChannel)
}

// NewNodeLister builds a node lister.
func NewNodeLister(kubeClient client.Interface, filter func(*apiv1.Node) bool, stopChannel <-chan struct{}) NodeLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "nodes", apiv1.NamespaceAll, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &apiv1.Node{}, time.Hour)
	nodeLister := v1lister.NewNodeLister(store)
	go reflector.Run(stopChannel)
	return &nodeListerImpl{
		nodeLister: nodeLister,
		filter:     filter,
	}
}

// List returns list of nodes.
func (l *nodeListerImpl) List() ([]*apiv1.Node, error) {
	var nodes []*apiv1.Node
	var err error

	nodes, err = l.nodeLister.List(labels.Everything())
	if err != nil {
		return []*apiv1.Node{}, err
	}

	if l.filter != nil {
		nodes = filterNodes(nodes, l.filter)
	}

	return nodes, nil
}

// Get returns the node with the given name.
func (l *nodeListerImpl) Get(name string) (*apiv1.Node, error) {
	node, err := l.nodeLister.Get(name)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func filterNodes(nodes []*apiv1.Node, predicate func(*apiv1.Node) bool) []*apiv1.Node {
	var filtered []*apiv1.Node
	for i := range nodes {
		if predicate(nodes[i]) {
			filtered = append(filtered, nodes[i])
		}
	}
	return filtered
}

// PodDisruptionBudgetLister lists pod disruption budgets.
type PodDisruptionBudgetLister interface {
	List() ([]*policyv1.PodDisruptionBudget, error)
}

// PodDisruptionBudgetListerImpl lists pod disruption budgets
type PodDisruptionBudgetListerImpl struct {
	pdbLister v1policylister.PodDisruptionBudgetLister
}

// List returns all pdbs
func (lister *PodDisruptionBudgetListerImpl) List() ([]*policyv1.PodDisruptionBudget, error) {
	return lister.pdbLister.List(labels.Everything())
}

// NewPodDisruptionBudgetLister builds a pod disruption budget lister.
func NewPodDisruptionBudgetLister(kubeClient client.Interface, stopchannel <-chan struct{}) PodDisruptionBudgetLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.PolicyV1().RESTClient(), "poddisruptionbudgets", apiv1.NamespaceAll, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &policyv1.PodDisruptionBudget{}, time.Hour)
	pdbLister := v1policylister.NewPodDisruptionBudgetLister(store)
	go reflector.Run(stopchannel)
	return &PodDisruptionBudgetListerImpl{
		pdbLister: pdbLister,
	}
}

// NewDaemonSetLister builds a daemonset lister.
func NewDaemonSetLister(kubeClient client.Interface, stopchannel <-chan struct{}) v1appslister.DaemonSetLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.AppsV1().RESTClient(), "daemonsets", apiv1.NamespaceAll, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &appsv1.DaemonSet{}, time.Hour)
	lister := v1appslister.NewDaemonSetLister(store)
	go reflector.Run(stopchannel)
	return lister
}

// NewReplicationControllerLister builds a replicationcontroller lister.
func NewReplicationControllerLister(kubeClient client.Interface, stopchannel <-chan struct{}) v1lister.ReplicationControllerLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "replicationcontrollers", apiv1.NamespaceAll, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &apiv1.ReplicationController{}, time.Hour)
	lister := v1lister.NewReplicationControllerLister(store)
	go reflector.Run(stopchannel)
	return lister
}

// NewJobLister builds a job lister.
func NewJobLister(kubeClient client.Interface, stopchannel <-chan struct{}) v1batchlister.JobLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.BatchV1().RESTClient(), "jobs", apiv1.NamespaceAll, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &batchv1.Job{}, time.Hour)
	lister := v1batchlister.NewJobLister(store)
	go reflector.Run(stopchannel)
	return lister
}

// NewReplicaSetLister builds a replicaset lister.
func NewReplicaSetLister(kubeClient client.Interface, stopchannel <-chan struct{}) v1appslister.ReplicaSetLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.AppsV1().RESTClient(), "replicasets", apiv1.NamespaceAll, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &appsv1.ReplicaSet{}, time.Hour)
	lister := v1appslister.NewReplicaSetLister(store)
	go reflector.Run(stopchannel)
	return lister
}

// NewStatefulSetLister builds a statefulset lister.
func NewStatefulSetLister(kubeClient client.Interface, stopchannel <-chan struct{}) v1appslister.StatefulSetLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.AppsV1().RESTClient(), "statefulsets", apiv1.NamespaceAll, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &appsv1.StatefulSet{}, time.Hour)
	lister := v1appslister.NewStatefulSetLister(store)
	go reflector.Run(stopchannel)
	return lister
}

// NewConfigMapListerForNamespace builds a configmap lister for the passed namespace (including all).
func NewConfigMapListerForNamespace(kubeClient client.Interface, stopchannel <-chan struct{},
	namespace string) v1lister.ConfigMapLister {
	listWatcher := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "configmaps", namespace, fields.Everything())
	store, reflector := cache.NewNamespaceKeyedIndexerAndReflector(listWatcher, &apiv1.ConfigMap{}, time.Hour)
	lister := v1lister.NewConfigMapLister(store)
	go reflector.Run(stopchannel)
	return lister
}
