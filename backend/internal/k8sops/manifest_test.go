package k8sops

import (
	"context"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
)

// installApplyPatchReactor works around a client-go testing gap: the fake
// dynamic client's generic ObjectReaction handles ApplyPatchType via
// strategicpatch.StrategicMergePatch, which needs a typed Go struct's json
// tags to know each field's merge strategy — it cannot look those up on
// *unstructured.Unstructured, so every apply-patch against an *existing*
// fake-tracked object errors with "unable to find api field". Only the
// create path (a real Create call, see applyOne) works against the stock
// fake reactor. Prepending this reactor replaces ApplyPatchType handling
// with a plain field merge over the existing object, good enough to
// exercise the "update an existing resource" path in tests.
func installApplyPatchReactor(t *testing.T, dynamicClient *dynamicfake.FakeDynamicClient) {
	t.Helper()
	dynamicClient.PrependReactor("patch", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchAction, ok := action.(k8stesting.PatchAction)
		if !ok || patchAction.GetPatchType() != types.ApplyPatchType {
			return false, nil, nil
		}
		tracker := dynamicClient.Tracker()
		existing, err := tracker.Get(action.GetResource(), action.GetNamespace(), patchAction.GetName())
		if err != nil {
			return true, nil, err
		}
		existingUnstructured, ok := existing.(*unstructured.Unstructured)
		if !ok {
			return true, nil, err
		}
		var patch map[string]interface{}
		if err := json.Unmarshal(patchAction.GetPatch(), &patch); err != nil {
			return true, nil, err
		}
		for k, v := range patch {
			existingUnstructured.Object[k] = v
		}
		if err := tracker.Update(action.GetResource(), existingUnstructured, action.GetNamespace()); err != nil {
			return true, nil, err
		}
		return true, existingUnstructured, nil
	})
}

func TestApplyManifest_CreateThenUpdate(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme)
	installApplyPatchReactor(t, dynamicClient)
	mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)

	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo-config
  namespace: demo-ns
data:
  key: value
`
	results, err := ApplyManifest(context.Background(), dynamicClient, mapper, manifest, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Operation != ApplyOperationCreated {
		t.Errorf("expected created, got %q (err=%q)", results[0].Operation, results[0].Error)
	}

	// Re-applying the same manifest should now report an update.
	results, err = ApplyManifest(context.Background(), dynamicClient, mapper, manifest, "", false, false)
	if err != nil {
		t.Fatalf("unexpected error on re-apply: %v", err)
	}
	if results[0].Operation != ApplyOperationUpdated {
		t.Errorf("expected updated, got %q (err=%q)", results[0].Operation, results[0].Error)
	}
}

func TestApplyManifest_DefaultNamespaceFallback(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme)
	mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)

	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: no-ns-config
`
	results, err := ApplyManifest(context.Background(), dynamicClient, mapper, manifest, "fallback-ns", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Operation != ApplyOperationCreated {
		t.Errorf("expected created, got %q (err=%q)", results[0].Operation, results[0].Error)
	}
	if results[0].Namespace != "fallback-ns" {
		t.Errorf("expected fallback-ns, got %q", results[0].Namespace)
	}
}

func TestApplyManifest_MissingNamespaceIsPerDocumentError(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme)
	mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)

	// Two documents: the first has no namespace and no default is given, so
	// it must fail on its own without preventing the second (valid) one from
	// still being applied.
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: needs-ns
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: has-ns
  namespace: explicit-ns
`
	results, err := ApplyManifest(context.Background(), dynamicClient, mapper, manifest, "", false, false)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Operation != ApplyOperationError {
		t.Errorf("expected error for first document, got %q", results[0].Operation)
	}
	if results[1].Operation != ApplyOperationCreated {
		t.Errorf("expected created for second document, got %q (err=%q)", results[1].Operation, results[1].Error)
	}
}

func TestApplyManifest_InvalidYAMLRejectedUpfront(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme)
	mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)

	if _, err := ApplyManifest(context.Background(), dynamicClient, mapper, "kind: [", "", false, false); err == nil {
		t.Fatal("expected an error for malformed YAML")
	}

	if _, err := ApplyManifest(context.Background(), dynamicClient, mapper, "metadata:\n  name: no-kind-or-version\n", "", false, false); err == nil {
		t.Fatal("expected an error for a document missing apiVersion/kind")
	}
}

func TestApplyManifest_DryRunDoesNotError(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme)
	mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme)

	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: preview-config
  namespace: demo-ns
`
	// The fake dynamic client tracker does not truly honor server-side
	// dry-run semantics (unlike a real API server), so this only checks that
	// dry-run requests are accepted and produce a sensible result — actual
	// no-op behavior is verified against a real cluster (see docs/workflow.md
	// smoke test notes).
	results, err := ApplyManifest(context.Background(), dynamicClient, mapper, manifest, "", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Operation != ApplyOperationCreated {
		t.Errorf("expected created, got %q (err=%q)", results[0].Operation, results[0].Error)
	}
}
