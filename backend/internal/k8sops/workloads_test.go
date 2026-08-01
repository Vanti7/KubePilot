package k8sops

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeDeployment(namespace, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}
}

func newFakeStatefulSet(namespace, name string, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
	}
}

func newFakeDaemonSet(namespace, name string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}

func TestScaleWorkload(t *testing.T) {
	ctx := context.Background()

	t.Run("deployment", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newFakeDeployment("default", "web", 1))
		if err := ScaleWorkload(ctx, clientset, "Deployment", "default", "web", 5); err != nil {
			t.Fatalf("ScaleWorkload: %v", err)
		}
		got, err := clientset.AppsV1().Deployments("default").Get(ctx, "web", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if got.Spec.Replicas == nil || *got.Spec.Replicas != 5 {
			t.Fatalf("replicas = %v, want 5", got.Spec.Replicas)
		}
	})

	t.Run("statefulset", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newFakeStatefulSet("default", "db", 1))
		if err := ScaleWorkload(ctx, clientset, "StatefulSet", "default", "db", 3); err != nil {
			t.Fatalf("ScaleWorkload: %v", err)
		}
		got, err := clientset.AppsV1().StatefulSets("default").Get(ctx, "db", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get statefulset: %v", err)
		}
		if got.Spec.Replicas == nil || *got.Spec.Replicas != 3 {
			t.Fatalf("replicas = %v, want 3", got.Spec.Replicas)
		}
	})

	t.Run("daemonset rejected", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newFakeDaemonSet("default", "agent"))
		err := ScaleWorkload(ctx, clientset, "DaemonSet", "default", "agent", 2)
		if err == nil {
			t.Fatal("expected an error scaling a DaemonSet, got nil")
		}
	})
}

func TestRestartWorkload(t *testing.T) {
	ctx := context.Background()

	t.Run("deployment", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newFakeDeployment("default", "web", 2))
		if err := RestartWorkload(ctx, clientset, "Deployment", "default", "web"); err != nil {
			t.Fatalf("RestartWorkload: %v", err)
		}
		got, err := clientset.AppsV1().Deployments("default").Get(ctx, "web", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if _, ok := got.Spec.Template.Annotations[restartedAtAnnotation]; !ok {
			t.Fatalf("expected %s annotation on pod template, got %v", restartedAtAnnotation, got.Spec.Template.Annotations)
		}
	})

	t.Run("daemonset", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newFakeDaemonSet("default", "agent"))
		if err := RestartWorkload(ctx, clientset, "DaemonSet", "default", "agent"); err != nil {
			t.Fatalf("RestartWorkload: %v", err)
		}
		got, err := clientset.AppsV1().DaemonSets("default").Get(ctx, "agent", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get daemonset: %v", err)
		}
		if _, ok := got.Spec.Template.Annotations[restartedAtAnnotation]; !ok {
			t.Fatalf("expected %s annotation on pod template, got %v", restartedAtAnnotation, got.Spec.Template.Annotations)
		}
	})

	t.Run("unsupported kind rejected", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		err := RestartWorkload(ctx, clientset, "Job", "default", "batch-job")
		if err == nil {
			t.Fatal("expected an error restarting an unsupported kind, got nil")
		}
	})
}
