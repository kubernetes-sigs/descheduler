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

package util

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"k8s.io/utils/exec"
)

// Utilities for dealing with conntrack

const NoConnectionToDelete = "0 flow entries have been deleted"

func IsIPv6(netIP net.IP) bool {
	return netIP != nil && netIP.To4() == nil
}

func IsIPv6String(ip string) bool {
	netIP := net.ParseIP(ip)
	return IsIPv6(netIP)
}

func parametersWithFamily(isIPv6 bool, parameters ...string) []string {
	if isIPv6 {
		parameters = append(parameters, "-f", "ipv6")
	}
	return parameters
}

// ClearUDPConntrackForIP uses the conntrack tool to delete the conntrack entries
// for the UDP connections specified by the given service IP
func ClearUDPConntrackForIP(execer exec.Interface, ip string) error {
	parameters := parametersWithFamily(IsIPv6String(ip), "-D", "--orig-dst", ip, "-p", "udp")
	err := ExecConntrackTool(execer, parameters...)
	if err != nil && !strings.Contains(err.Error(), NoConnectionToDelete) {
		// TODO: Better handling for deletion failure. When failure occur, stale udp connection may not get flushed.
		// These stale udp connection will keep black hole traffic. Making this a best effort operation for now, since it
		// is expensive to baby-sit all udp connections to kubernetes services.
		return fmt.Errorf("error deleting connection tracking state for UDP service IP: %s, error: %v", ip, err)
	}
	return nil
}

// ExecConntrackTool executes the conntrack tool using the given parameters
func ExecConntrackTool(execer exec.Interface, parameters ...string) error {
	conntrackPath, err := execer.LookPath("conntrack")
	if err != nil {
		return fmt.Errorf("error looking for path of conntrack: %v", err)
	}
	output, err := execer.Command(conntrackPath, parameters...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("conntrack command returned: %q, error message: %s", string(output), err)
	}
	return nil
}

// ClearUDPConntrackForPort uses the conntrack tool to delete the conntrack entries
// for the UDP connections specified by the port.
// When a packet arrives, it will not go through NAT table again, because it is not "the first" packet.
// The solution is clearing the conntrack. Known issues:
// https://github.com/docker/docker/issues/8795
// https://github.com/kubernetes/kubernetes/issues/31983
func ClearUDPConntrackForPort(execer exec.Interface, port int, isIPv6 bool) error {
	if port <= 0 {
		return fmt.Errorf("Wrong port number. The port number must be greater than zero")
	}
	parameters := parametersWithFamily(isIPv6, "-D", "-p", "udp", "--dport", strconv.Itoa(port))
	err := ExecConntrackTool(execer, parameters...)
	if err != nil && !strings.Contains(err.Error(), NoConnectionToDelete) {
		return fmt.Errorf("error deleting conntrack entries for UDP port: %d, error: %v", port, err)
	}
	return nil
}

// ClearUDPConntrackForPeers uses the conntrack tool to delete the conntrack entries
// for the UDP connections specified by the {origin, dest} IP pair.
func ClearUDPConntrackForPeers(execer exec.Interface, origin, dest string) error {
	parameters := parametersWithFamily(IsIPv6String(origin), "-D", "--orig-dst", origin, "--dst-nat", dest, "-p", "udp")
	err := ExecConntrackTool(execer, parameters...)
	if err != nil && !strings.Contains(err.Error(), NoConnectionToDelete) {
		// TODO: Better handling for deletion failure. When failure occur, stale udp connection may not get flushed.
		// These stale udp connection will keep black hole traffic. Making this a best effort operation for now, since it
		// is expensive to baby sit all udp connections to kubernetes services.
		return fmt.Errorf("error deleting conntrack entries for UDP peer {%s, %s}, error: %v", origin, dest, err)
	}
	return nil
}
