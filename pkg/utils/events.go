package utils

import (
	"context"

	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/events"
)

func GetRecorderAndBroadcaster(ctx context.Context, clientset clientset.Interface) (events.EventBroadcasterAdapter, events.EventRecorder) {
	eventBroadcaster := events.NewEventBroadcasterAdapter(clientset)
	eventBroadcaster.StartRecordingToSink(ctx.Done())
	eventRecorder := eventBroadcaster.NewRecorder("sigs.k8s.io.descheduler")
	return eventBroadcaster, eventRecorder
}
