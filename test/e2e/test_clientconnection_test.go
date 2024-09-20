package e2e

import (
	"context"
	"os"
	"testing"

	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
)

func TestClientConnectionConfiguration(t *testing.T) {
	ctx := context.Background()
	clientConnection := componentbaseconfig.ClientConnectionConfiguration{
		Kubeconfig: os.Getenv("KUBECONFIG"),
		QPS:        50,
		Burst:      100,
	}
	clientSet, err := client.CreateClient(clientConnection, "")
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	s, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	s.Client = clientSet
	evictionPolicyGroupVersion, err := eutils.SupportEviction(s.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		t.Errorf("Error when checking support for eviction: %v", err)
	}
	if err := descheduler.RunDeschedulerStrategies(ctx, s, &deschedulerapi.DeschedulerPolicy{}, evictionPolicyGroupVersion); err != nil {
		t.Errorf("Error running descheduler strategies: %+v", err)
	}
}
