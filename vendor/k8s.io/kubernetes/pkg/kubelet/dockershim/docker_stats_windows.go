// +build windows

/*
Copyright 2017 The Kubernetes Authors.

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

package dockershim

import (
	"time"

	runtimeapi "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// ContainerStats returns stats for a container stats request based on container id.
func (ds *dockerService) ContainerStats(containerID string) (*runtimeapi.ContainerStats, error) {
	containerStats, err := ds.getContainerStats(containerID)
	if err != nil {
		return nil, err
	}
	return containerStats, nil
}

// ListContainerStats returns stats for a list container stats request based on a filter.
func (ds *dockerService) ListContainerStats(containerStatsFilter *runtimeapi.ContainerStatsFilter) ([]*runtimeapi.ContainerStats, error) {
	filter := &runtimeapi.ContainerFilter{}

	if containerStatsFilter != nil {
		filter.Id = containerStatsFilter.Id
		filter.PodSandboxId = containerStatsFilter.PodSandboxId
		filter.LabelSelector = containerStatsFilter.LabelSelector
	}

	containers, err := ds.ListContainers(filter)
	if err != nil {
		return nil, err
	}

	var stats []*runtimeapi.ContainerStats
	for _, container := range containers {
		containerStats, err := ds.getContainerStats(container.Id)
		if err != nil {
			return nil, err
		}

		stats = append(stats, containerStats)
	}

	return stats, nil
}

func (ds *dockerService) getContainerStats(containerID string) (*runtimeapi.ContainerStats, error) {
	statsJSON, err := ds.client.GetContainerStats(containerID)
	if err != nil {
		return nil, err
	}

	containerJSON, err := ds.client.InspectContainerWithSize(containerID)
	if err != nil {
		return nil, err
	}

	status, err := ds.ContainerStatus(containerID)
	if err != nil {
		return nil, err
	}

	dockerStats := statsJSON.Stats
	timestamp := time.Now().UnixNano()
	containerStats := &runtimeapi.ContainerStats{
		Attributes: &runtimeapi.ContainerAttributes{
			Id:          containerID,
			Metadata:    status.Metadata,
			Labels:      status.Labels,
			Annotations: status.Annotations,
		},
		Cpu: &runtimeapi.CpuUsage{
			Timestamp: timestamp,
			// have to multiply cpu usage by 100 since docker stats units is in 100's of nano seconds for Windows
			// see https://github.com/moby/moby/blob/v1.13.1/api/types/stats.go#L22
			UsageCoreNanoSeconds: &runtimeapi.UInt64Value{Value: dockerStats.CPUStats.CPUUsage.TotalUsage * 100},
		},
		Memory: &runtimeapi.MemoryUsage{
			Timestamp:       timestamp,
			WorkingSetBytes: &runtimeapi.UInt64Value{Value: dockerStats.MemoryStats.PrivateWorkingSet},
		},
		WritableLayer: &runtimeapi.FilesystemUsage{
			Timestamp: timestamp,
			UsedBytes: &runtimeapi.UInt64Value{Value: uint64(*containerJSON.SizeRw)},
		},
	}
	return containerStats, nil
}
