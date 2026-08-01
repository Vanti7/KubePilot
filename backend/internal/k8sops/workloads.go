// Package k8sops performs write operations against a live Kubernetes
// cluster (scale, restart — deploy/Helm-upgrade later). It is the first
// place in this backend that mutates a cluster rather than only reading it,
// so it is kept separate from internal/collector (read-only) and takes a
// plain kubernetes.Interface rather than a *collector.KubernetesCollector,
// so it can be tested with a fake clientset.
package k8sops

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// restartedAtAnnotation is the same annotation key `kubectl rollout restart`
// uses, so external tooling and `kubectl rollout status` keep working.
const restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"

// ScaleWorkload sets a Deployment or StatefulSet's replica count via a JSON
// merge patch on spec.replicas. DaemonSets have no replica count; callers
// should reject that kind before reaching here, but it is rejected here too
// as a second guard.
func ScaleWorkload(ctx context.Context, clientset kubernetes.Interface, kind, namespace, name string, replicas int32) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))

	switch kind {
	case "Deployment":
		_, err := clientset.AppsV1().Deployments(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	case "StatefulSet":
		_, err := clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("scale is not supported for kind %q", kind)
	}
}

// RestartWorkload triggers a rolling restart the same way `kubectl rollout
// restart` does: patch the pod template with a fresh restartedAt
// annotation, forcing a new revision without changing anything else.
func RestartWorkload(ctx context.Context, clientset kubernetes.Interface, kind, namespace, name string) error {
	patch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{
						restartedAtAnnotation: time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	switch kind {
	case "Deployment":
		_, err := clientset.AppsV1().Deployments(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	case "StatefulSet":
		_, err := clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	case "DaemonSet":
		_, err := clientset.AppsV1().DaemonSets(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("restart is not supported for kind %q", kind)
	}
}
