// +build acceptance baremetal allocations

package v1

import (
	"testing"

	"github.com/gophercloud/gophercloud/acceptance/clients"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/allocations"
	"github.com/gophercloud/gophercloud/pagination"
	th "github.com/gophercloud/gophercloud/testhelper"
)

func TestAllocationsCreateDestroy(t *testing.T) {
	clients.RequireLong(t)

	client, err := clients.NewBareMetalV1Client()
	th.AssertNoErr(t, err)

	client.Microversion = "1.52"

	allocation, err := CreateAllocation(t, client)
	th.AssertNoErr(t, err)
	defer DeleteAllocation(t, client, allocation)

	found := false
	err = allocations.List(client, allocations.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		allocationList, err := allocations.ExtractAllocations(page)
		if err != nil {
			return false, err
		}

		for _, a := range allocationList {
			if a.UUID == allocation.UUID {
				found = true
				return true, nil
			}
		}

		return false, nil
	})
	th.AssertNoErr(t, err)
	th.AssertEquals(t, found, true)
}
