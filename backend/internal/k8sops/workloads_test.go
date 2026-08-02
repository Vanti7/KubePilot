package k8sops

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

func newFakeDeploymentWithContainers(namespace, name string, containers ...corev1.Container) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: containers},
			},
		},
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

func TestSetContainerImage(t *testing.T) {
	ctx := context.Background()

	t.Run("single container", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newFakeDeploymentWithContainers("default", "web",
			corev1.Container{Name: "app", Image: "nginx:1.24"},
		))
		if err := SetContainerImage(ctx, clientset, "Deployment", "default", "web", "app", "nginx:1.25"); err != nil {
			t.Fatalf("SetContainerImage: %v", err)
		}
		got, err := clientset.AppsV1().Deployments("default").Get(ctx, "web", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if len(got.Spec.Template.Spec.Containers) != 1 || got.Spec.Template.Spec.Containers[0].Image != "nginx:1.25" {
			t.Fatalf("containers = %+v, want [app:nginx:1.25]", got.Spec.Template.Spec.Containers)
		}
	})

	// The critical regression this guards against: a naive JSON merge patch
	// on spec.template.spec.containers replaces the whole list, wiping out
	// every container besides the one being patched. A strategic merge patch
	// must merge by container name instead.
	t.Run("multi container — sidecar untouched", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(newFakeDeploymentWithContainers("default", "web",
			corev1.Container{Name: "app", Image: "nginx:1.24"},
			corev1.Container{Name: "sidecar", Image: "envoy:1.30"},
		))
		if err := SetContainerImage(ctx, clientset, "Deployment", "default", "web", "app", "nginx:1.25"); err != nil {
			t.Fatalf("SetContainerImage: %v", err)
		}
		got, err := clientset.AppsV1().Deployments("default").Get(ctx, "web", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if len(got.Spec.Template.Spec.Containers) != 2 {
			t.Fatalf("expected 2 containers to survive, got %+v", got.Spec.Template.Spec.Containers)
		}
		byName := map[string]string{}
		for _, c := range got.Spec.Template.Spec.Containers {
			byName[c.Name] = c.Image
		}
		if byName["app"] != "nginx:1.25" {
			t.Errorf("app image = %q, want nginx:1.25", byName["app"])
		}
		if byName["sidecar"] != "envoy:1.30" {
			t.Errorf("sidecar image = %q, want unchanged envoy:1.30, got wiped or altered", byName["sidecar"])
		}
	})

	t.Run("statefulset", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
			Spec: appsv1.StatefulSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "db", Image: "postgres:15"}}},
				},
			},
		})
		if err := SetContainerImage(ctx, clientset, "StatefulSet", "default", "db", "db", "postgres:16"); err != nil {
			t.Fatalf("SetContainerImage: %v", err)
		}
		got, err := clientset.AppsV1().StatefulSets("default").Get(ctx, "db", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get statefulset: %v", err)
		}
		if got.Spec.Template.Spec.Containers[0].Image != "postgres:16" {
			t.Fatalf("image = %q, want postgres:16", got.Spec.Template.Spec.Containers[0].Image)
		}
	})

	t.Run("unsupported kind rejected", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		err := SetContainerImage(ctx, clientset, "Job", "default", "batch-job", "app", "app:2")
		if err == nil {
			t.Fatal("expected an error setting image on an unsupported kind, got nil")
		}
	})
}
